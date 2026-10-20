// Wails v2 绑定桥的薄封装：不依赖 wails CLI 生成物，
// 直接走 window.go.<package>.<Struct>.<Method> 与 window.runtime。
// 错误约定：Go 绑定方法返回 (result, error)，到 JS 侧为 Promise，
// reject 的 Error.message 直接展示在 UI 横幅（FR-12：缺资源/错误不静默）。

/** App 绑定结构所在路径：window.go.main.App */
const app = () => (window as any).go?.main?.App;

/** 启动自检：验证 Go 绑定桥可用，返回版本串。 */
export async function pingVersion(): Promise<string> {
  const a = app();
  if (!a) throw new Error("Go 绑定桥未就绪（window.go 不存在）");
  return a.Version();
}

export const api = {
  pingVersion,
};
