//go:build darwin && race

// seatbelt_race_on.go——race 构建判定（定义面=使用面：seatbelt_goruntime_contract_test.go
// 实证步消费）。TSan 载体在受限 Seatbelt 内 CHECK failed（sanitizer_mac.cpp——边界拒绝
// TSan 运行时所需 mach/sysctl 面=fail-closed 实证），实证步在非 race 构建执行。
package sandbox

const raceInstrumented = true
