// Wails v2 绑定桥的薄封装：不依赖 wails CLI 生成物，
// 直接走 window.go.main.App.<Method> 与 window.runtime。
// 错误约定：Go 绑定 reject 的 Error.message 直接展示在 UI 横幅
// （FR-12：缺资源/错误不静默）。

const app = () => (window as any).go?.main?.App;

/** 内部库 DTO —— 与 internal/library/library.go 的 JSON 结构对应。 */
export interface CGEvent {
  id: string;
  index: number;
  title: string;
  thumb: string;
  script: string;
  label: string;
  pages: string[];
}
export interface SceneEvent {
  id: string;
  title: string;
  script: string;
}
export interface ChapterEvent {
  id: string;
  index: number;
  title: string;
  script: string;
  scene: string;
  label_pos: number;
  name: string;
}
export interface LibraryDTO {
  root: string;
  game?: string;
  generated_at: string;
  cgs: CGEvent[];
  scenes: SceneEvent[];
  chapters?: ChapterEvent[];
}
export interface ConfDTO {
  root: string;
  library: string;
  auto: boolean;
  conf_missing: boolean;
}

/** 卡片网格统一事件视图（kind 定位三个来源）。 */
export type CardItem =
  | { kind: "cg"; id: string; title: string; note: string; ok: true }
  | { kind: "scene"; id: string; title: string; note: ""; ok: true }
  | { kind: "chapter"; id: string; title: string; note: string; ok: true }
  | { kind: string; id: string; title: string; note: string; ok: false };

/** 未就绪异常：绑定桥没起来（Wails 注入失败）时统一可读报错。 */
function bridge() {
  const a = app();
  if (!a) throw new Error("Go 绑定桥未就绪（window.go 不存在，请用 wails build 产物）");
  return a;
}

export const api = {
  pingVersion: () => bridge().Version() as Promise<string>,
  getConf: () => bridge().GetConf() as Promise<ConfDTO>,
  pickDirectory: () => bridge().PickDirectory() as Promise<string>,
  setRoot: (root: string) => bridge().SetRoot(root) as Promise<LibraryDTO>,
  getLibrary: () => bridge().GetLibrary() as Promise<LibraryDTO>,
  rescan: () => bridge().Rescan() as Promise<LibraryDTO>,
  getCover: (kind: string, id: string) =>
    bridge().GetCover(kind, id) as Promise<string>,
};
