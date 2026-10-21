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

  // ---------- M3 回放会话（desktop/replay.go 绑定方法） ----------
  openEvent: (kind: string, id: string) =>
    bridge().OpenEvent(kind, id) as Promise<SessionMeta>,
  // 高频轮询：Go 端墙钟推进 + 同帧指纹缓存，前端 rAF/interval 驱动皆可
  getFrame: () => bridge().GetFrame() as Promise<FrameDTO>,
  play: () => bridge().Play() as Promise<void>,
  pause: () => bridge().Pause() as Promise<void>,
  togglePlay: () => bridge().TogglePlay() as Promise<boolean>,
  inputClick: () => bridge().InputClick() as Promise<void>,
  toggleAuto: () => bridge().ToggleAuto() as Promise<boolean>,
  setMuted: (m: boolean) => bridge().SetMuted(m) as Promise<void>,
  seekTo: (ms: number) => bridge().SeekTo(Math.round(ms)) as Promise<void>,
  exitReplay: () => bridge().ExitReplay() as Promise<void>,
  screenshot: () => bridge().Screenshot() as Promise<string>,
};

// ---------- M3 回放 DTO（与 desktop/replay.go 对应） ----------

export interface SessionMeta {
  kind: string;
  id: string;
  title: string;
  stage_w: number; // 目前固定 1600×1200
  stage_h: number;
  op_count: number;
  text_count: number;
  duration_ms: number; // Show 台本时钟最大值（进度参考刻度），可能为 0（无 wait/wt 事件）
}

export interface LayerDTO {
  layer: string;
  ref: string;
  visible: boolean;
  x: number;
  y: number;
  opacity: number;
  z: number;
  missing: boolean;
}

export interface LineDTO {
  speaker: string;
  text: string;
}

export interface BoxDTO {
  layer: string;
  x: number;
  y: number;
  w: number;
  h: number;
  has_geom: boolean;
  visible: boolean;
  frame_color: number[];
  frame_opacity: number;
  font_size: number;
  line_spacing: number;
  lines: LineDTO[];
}

export interface FrameDTO {
  jpeg: string; // dataURL，同画面指纹时为 ""
  now: number;
  dur: number;
  playing: boolean;
  done: boolean;
  waiting: boolean;
  auto: boolean;
  muted: boolean;
  quake_x: number;
  quake_y: number;
  cached: boolean;
  missing: string[]; // 缺素材 ref（FR-12：常驻红横幅，不静默）
  layers: LayerDTO[]; // 调试抽屉 D
  texts: BoxDTO[]; // 调试抽屉 D（文本框状态）
}
