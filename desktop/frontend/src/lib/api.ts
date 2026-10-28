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
/** 解包预览（desktop/unpack.go UnpackPlanDTO）。 */
export interface UnpackPlanDTO {
  engine_id: string;
  kind: string; // loose=已解包目录 / xp3=XP3 归档
  archives: string[];
  scenario_files: number;
  labels: number; // 预计的 CG 事件数
  refs: number; // 预计引用到的素材数
  videos: number; // 其中影片（OP/ED）条目数
  video_bytes: number; // 影片原始体积（估算）
  /** 该源已解包目录里现有的影片数（>0 = 之前提过，重解不会删） */
  imported_videos: number;
  /** 该源之前已解包过（产物目录非空） */
  imported: boolean;
  /** 这次解包预计产出的字节数/文件数 */
  est_bytes: number;
  est_files: number;
  encrypted: number;
  notes: string[];
  unsupported: boolean;
  reason: string;
}
/** 解包导入结果（desktop/unpack.go UnpackImportDTO）。 */
export interface UnpackImportDTO {
  dest: string;
  engine_id: string;
  events: number;
  files: number;
  bytes: number;
  missing: string[];
  encrypted: string[];
  warnings: string[];
  /** 按「不提取影片」选项跳过的数量 */
  skipped_video?: number;
  /** 已存在且大小一致、直接复用的文件数（增量解包） */
  reused?: number;
  game_id: string;
  /** false = 已解包但事件库接入失败（例如扫描不到事件），产物仍保留在 dest。 */
  connected: boolean;
  reason?: string;
  hint?: string;
  library?: LibraryDTO;
}

export interface ConfDTO {
  root: string;
  library: string;
  auto: boolean;
  conf_missing: boolean;
}
export interface GameDTO {
  id: string;
  title: string;
  root: string;
  library?: string;
  engine?: string;
  added_at: string;
  last_played_at?: string;
}
export interface GamesDTO {
  active: string;
  games: GameDTO[];
}
/** 中心库索引规模（desktop/catalog.go CatalogStatsDTO）。 */
export interface CatalogStatsDTO {
  games: number;
  events: number;
  text_lines: number;
  error?: string;
}

// ---------- 跨游戏视图 / 台词检索（第三阶段） ----------

/** 跨游戏事件卡片（desktop/catalog.go CatalogEventDTO）。 */
export interface CatalogEventDTO {
  game_id: string;
  game_title: string;
  game_root: string;
  kind: string;
  id: string;
  title: string;
  ord: number;
  pages: number;
  has_cover: boolean;
}
export interface CatalogEventsDTO {
  events: CatalogEventDTO[];
  warnings: string[];
}
/** 台词命中（desktop/catalog.go CatalogTextHitDTO）。 */
export interface CatalogTextHitDTO {
  line_id: number;
  game_id: string;
  game_title: string;
  kind: string;
  event_id: string;
  t_ms: number;
  speaker: string;
  text: string;
}
export interface CatalogTextDTO {
  hits: CatalogTextHitDTO[];
  warnings: string[];
  indexed: boolean; // false = 台词尚未抽取，0 条结果不代表没搜到
}
export interface CatalogIndexResultDTO {
  game_id: string;
  events: number;
  lines: number;
  warnings: string[];
}
/** 最近播放（desktop/catalog.go CatalogRecentDTO）。 */
export interface CatalogRecentDTO {
  game_id: string;
  game_title: string;
  kind: string;
  event_id: string;
  title: string;
  played_at: string;
  play_count: number;
}
/** 收藏（desktop/catalog.go FavoriteDTO）。rating 0 = 未评分。 */
export interface FavoriteDTO {
  item_type: "game" | "event" | "character" | string;
  game_id: string;
  item_id: string;
  rating: number;
  note: string;
  title: string;
  game_title: string;
  created_at: string;
  updated_at: string;
}
/** 说话人聚合（desktop/catalog.go SpeakerDTO）——character 级收藏前置。 */
export interface SpeakerDTO {
  game_id: string;
  game_title: string;
  name: string;
  lines: number;
  events: number;
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
  openPath: (path: string) => bridge().OpenPath(path) as Promise<void>,
  listGames: () => bridge().ListGames() as Promise<GamesDTO>,
  addGame: (root: string) => bridge().AddGame(root) as Promise<LibraryDTO>,
  unpackPreview: (path: string) =>
    bridge().UnpackPreview(path) as Promise<UnpackPlanDTO>,
  unpackImport: (path: string, skipVideo: boolean, force: boolean) =>
    bridge().UnpackImport(path, skipVideo, force) as Promise<UnpackImportDTO>,
  unpackImportDecrypt: (path: string, scheme: string, key: number, skipVideo: boolean, force: boolean) =>
    bridge().UnpackImportDecrypt(path, scheme, key, skipVideo, force) as Promise<UnpackImportDTO>,
  setActiveGame: (id: string) =>
    bridge().SetActiveGame(id) as Promise<LibraryDTO>,
  catalogStats: () => bridge().CatalogStats() as Promise<CatalogStatsDTO>,
  // ---------- 跨游戏视图（全部游戏卡片墙 / 台词检索） ----------
  catalogEvents: (query: string, limit: number) =>
    bridge().CatalogEvents(query, limit) as Promise<CatalogEventsDTO>,
  catalogSearchText: (query: string, limit: number) =>
    bridge().CatalogSearchText(query, limit) as Promise<CatalogTextDTO>,
  catalogTextCounts: () =>
    bridge().CatalogTextCounts() as Promise<Record<string, number>>,
  catalogIndexText: (gameID: string) =>
    bridge().CatalogIndexText(gameID) as Promise<CatalogIndexResultDTO>,
  catalogRecent: (limit: number) =>
    bridge().CatalogRecent(limit) as Promise<CatalogRecentDTO[]>,
  /** 清理 N 天前的最近播放（0 = 默认 3 天/72h），返回删除条数。 */
  catalogPruneRecent: (days: number) =>
    bridge().CatalogPruneRecent(days) as Promise<number>,
  // ---------- 收藏（event 级；game/character 级 API 已就绪、UI 后续） ----------
  addFavorite: (itemType: string, gameID: string, itemID: string, rating: number, note: string) =>
    bridge().AddFavorite(itemType, gameID, itemID, rating, note) as Promise<void>,
  removeFavorite: (itemType: string, gameID: string, itemID: string) =>
    bridge().RemoveFavorite(itemType, gameID, itemID) as Promise<void>,
  listFavorites: (itemType: string) =>
    bridge().ListFavorites(itemType) as Promise<FavoriteDTO[]>,
  catalogSpeakers: (gameID: string) =>
    bridge().CatalogSpeakers(gameID) as Promise<SpeakerDTO[]>,
  getCoverForGame: (gameID: string, kind: string, id: string) =>
    bridge().GetCoverForGame(gameID, kind, id) as Promise<string>,
  // 兼容旧绑定；等价于 addGame
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
  // ---------- 导出任务中心（异步：StartExport 起任务，事件推进度/结果） ----------
  startExport: (format: string) => bridge().StartExport(format) as Promise<void>,
  cancelExport: () => bridge().CancelExport() as Promise<void>,
};

/** 导出进度（Go 端 ExportProgress，事件 export:progress）。 */
export interface ExportProgress {
  frame: number;
  total: number;
  percent: number;
  format: string;
}

// ---------- M3 回放 DTO（与 desktop/replay.go 对应） ----------

export interface SessionMeta {
  kind: string;
  id: string;
  title: string;
  game_id: string; // 收藏/跨游戏定位用
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

/** 导出结果（desktop/export.go ExportResult）。 */
export interface ExportResult {
  path: string;
  format: "gif" | "mp4";
  frames: number;
  fps: number;
  width: number;
  height: number;
  duration_ms: number;
  missing: string[];
  note: string;
}

/**
 * 订阅导出任务三事件（Go 端 wruntime.EventsEmit）。
 * 返回退订函数；没有 window.runtime（非 Wails 环境）时返回 no-op。
 *   - export:progress → ExportProgress（帧/总数/百分比）
 *   - export:done     → ExportResult（路径 + 摘要）
 *   - export:failed   → { message }（含「导出已取消」）
 */
export function onExportEvents(handlers: {
  onProgress?: (p: ExportProgress) => void;
  onDone?: (r: ExportResult) => void;
  onFailed?: (message: string) => void;
}): () => void {
  const rt = (window as any).runtime;
  if (!rt?.EventsOn) return () => {};
  const offs: unknown[] = [];
  if (handlers.onProgress) offs.push(rt.EventsOn("export:progress", handlers.onProgress));
  if (handlers.onDone) offs.push(rt.EventsOn("export:done", handlers.onDone));
  if (handlers.onFailed) {
    offs.push(
      rt.EventsOn("export:failed", (e: any) =>
        handlers.onFailed?.(e?.message ?? String(e)),
      ),
    );
  }
  return () => {
    for (const off of offs) {
      if (typeof off === "function") (off as () => void)();
    }
  };
}
