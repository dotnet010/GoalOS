// bypass_shared_test.go——TC-RT-001 族共享先红形态（R-1452：SKIP=合法先红/FAIL=非法；
// R-571 测试先行；断言来源=12 清单 F 节 TC-RT-001a/b/c 行）。
// 旁路测试纪律（TC-RT-002 联动）：能力代理拒绝≠边界拒绝——分开计数（BypassCounters）。
package runtime

import "testing"

// runBypassProbe 三平台共享：先红=受限档 Provider 未注册（任务 5.1/5.2/5.3 收敛前
// 注册表为空——骨架纪律 R-1468）→Skip 登记先红；Provider 存在但探针断言未接线→FAIL
// （反虚假绿：注册与探针断言必须同任务完成）；转绿后=真实旁路断言。
//
// 转绿断言规格（TC 原文——W5-6 接线时逐字实现，禁止缩水）：
// 沙箱内进程绕过能力代理直接 open（工作区外路径）/connect（出站）→断言被 OS 边界拒绝
// （非代理拒绝误计——BypassCounters 分离计数：RecordProxyRefusal 不得提升 BypassPasses）。
func runBypassProbe(t *testing.T, tc string) {
	t.Helper()
	reg := NewProviderRegistry()
	_, err := reg.AcquireForTier("T1")
	if err != nil {
		t.Skipf("%s 先红（W1 注册，R-1452 合法先红形态）：受限档 Provider 未注册（%v）——转绿=W5-6 Provider 收敛后执行真实旁路断言", tc, err)
	}
	// Provider 已注册——探针断言必须与注册同任务接线（反虚假绿）：
	// 当前无探针实现（探针负载=direct open/connect 绕代理——经 Provider 执行），
	// 注册即失败直到断言落地。
	t.Fatalf("%s 转绿窗口：Provider 已注册但旁路探针断言未接线——任务 5.1/5.2/5.3 同一验收必须完成：direct open（工作区外）+connect（出站）被 OS 边界拒绝+BypassCounters 分离计数", tc)
}
