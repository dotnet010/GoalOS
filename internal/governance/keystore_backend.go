// keystore_backend.go——KeystoreBackend 骨架（06 §X.8：DPAPI/Keychain/信创机密服务
// 后端=预留——契约现定不实现，R-1505 收窄；R-1531 四方法骨架；R-1468 骨架纪律：
// 统一返回包级 ErrNotImplemented）。
package governance

import "errors"

// ErrKeystoreNotImplemented KeystoreBackend 骨架占位（R-1468 统一纪律）。
var ErrKeystoreNotImplemented = errors.New("governance: KeystoreBackend 未实现（骨架——R-1531/R-1468）")

// KeystoreBackend OS 级密钥托管后端契约（06 §X.8——DPAPI/Keychain/信创机密服务；
// 预留：签名密钥族当前=secrets.key keyring 唯一载体，本接口=未来产品化入口）。
// 四方法骨架——契约现定不实现。
type KeystoreBackend interface {
	// Get 读取托管密钥材料。
	Get(key string) ([]byte, error)
	// Set 写入托管密钥材料。
	Set(key string, value []byte) error
	// Delete 删除托管密钥材料。
	Delete(key string) error
	// List 枚举托管键。
	List() ([]string, error)
}

// keystoreBackendSkeleton 骨架实现（统一 ErrKeystoreNotImplemented——R-1468）。
type keystoreBackendSkeleton struct{}

// NewKeystoreBackendSkeleton 构造骨架（预留注册点——当前不注册任何生产路径）。
func NewKeystoreBackendSkeleton() KeystoreBackend { return keystoreBackendSkeleton{} }

func (keystoreBackendSkeleton) Get(string) ([]byte, error)     { return nil, ErrKeystoreNotImplemented }
func (keystoreBackendSkeleton) Set(string, []byte) error       { return ErrKeystoreNotImplemented }
func (keystoreBackendSkeleton) Delete(string) error            { return ErrKeystoreNotImplemented }
func (keystoreBackendSkeleton) List() ([]string, error)        { return nil, ErrKeystoreNotImplemented }
