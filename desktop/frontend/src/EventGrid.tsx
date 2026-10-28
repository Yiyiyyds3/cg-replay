import { useEffect, useMemo, useRef, useState } from "react";
import {
  Clock,
  Database,
  FolderOpen,
  Gamepad2,
  Loader2,
  PackageOpen,
  PanelLeftClose,
  PanelLeftOpen,
  Pencil,
  Plus,
  RefreshCw,
  Search,
  Settings,
  Ship,
  Star,
  Users,
} from "lucide-react";
import { SettingsDialog } from "./SettingsDialog";
import {
  api,
  type CardItem,
  type CatalogEventDTO,
  type CatalogRecentDTO,
  type CatalogStatsDTO,
  type FavoriteDTO,
  type GameDTO,
  type GamesDTO,
  type LibraryDTO,
  type SpeakerDTO,
  type UnpackImportDTO,
  type UnpackPlanDTO,
} from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";

type Tab = "all" | "cg" | "scene" | "chapter";

type Status =
  | { state: "loading" }
  | { state: "setup" } // 需要 conf.Root
  | { state: "ready" }
  | { state: "error"; message: string };

const SIDEBAR_KEY = "ucgr.sidebarCollapsed";
const RECENT_LIMIT = 12; // 「全部」页签最近播放取多少条（清理后同量刷新）
const AUTOPRUNE_KEY = "ucgr.autoPruneRecent"; // 启动自动清理 72h 前最近播放的开关（默认关）
// 解包时是否提取 OP/ED 影片。默认开（提取）；"0" = 用户选择跳过。
const EXTRACT_VIDEO_KEY = "ucgr.extractVideo";
const PRUNE_DAYS = 3; // 冷归档窗口（与 UI 文案一致）

// M2 事件网格（卡墙）。M3：卡片点击 → App 路由切回放页。
export default function EventGrid({
  onOpen,
}: {
  onOpen: (kind: string, id: string, title: string) => void;
}) {
  const [status, setStatus] = useState<Status>({ state: "loading" });
  const [lib, setLib] = useState<LibraryDTO | null>(null);
  const [games, setGames] = useState<GamesDTO | null>(null);
  const [catalog, setCatalog] = useState<CatalogStatsDTO | null>(null);
  const [tab, setTab] = useState<Tab>("cg");
  const [q, setQ] = useState("");
  const [busy, setBusy] = useState(false);
  const [busyGame, setBusyGame] = useState("");
  // ---------- 「全部」页签：跨游戏卡片墙 / 角色 / 收藏 / 最近播放 ----------
  const [allEvents, setAllEvents] = useState<CatalogEventDTO[] | null>(null);
  const [allWarnings, setAllWarnings] = useState<string[]>([]);
  const [textIndexed, setTextIndexed] = useState(true);
  const [recentItems, setRecentItems] = useState<CatalogRecentDTO[]>([]);
  const [pruningRecent, setPruningRecent] = useState(false); // 清理 72h 前记录进行中
  const [unpackPlan, setUnpackPlan] = useState<UnpackPlanDTO | null>(null); // 解包预览
  const [unpackPath, setUnpackPath] = useState(""); // 待解包的源路径
  const [unpacking, setUnpacking] = useState(false); // 解包进行中
  const [unpackResult, setUnpackResult] = useState<UnpackImportDTO | null>(null); // 已解包但未接入
  const [unpackScheme, setUnpackScheme] = useState(""); // XP3 解密方案；空 = 不解密
  const [unpackKey, setUnpackKey] = useState("0"); // xor 固定密钥（0-255）
  // 提取影片的偏好：默认开（提取）。落 localStorage，下次解包沿用上次选择。
  const [extractVideo, setExtractVideo] = useState(() => {
    try {
      return window.localStorage.getItem(EXTRACT_VIDEO_KEY) !== "0";
    } catch {
      return true;
    }
  });
  // 点「开始解包」后、真解之前先问一次（有影片时）。
  const [askVideo, setAskVideo] = useState(false);
  const [settingsOpen, setSettingsOpen] = useState(false); // 设置弹层
  const [forceReextract, setForceReextract] = useState(false); // 重读时「强制重解」

  // 改「提取影片」偏好：状态 + localStorage 一起写（弹层与面板勾选框共用）。
  const setExtractVideoPref = (next: boolean) => {
    setExtractVideo(next);
    try {
      window.localStorage.setItem(EXTRACT_VIDEO_KEY, next ? "1" : "0");
    } catch {
      /* localStorage 不可用就只影响本次会话 */
    }
  };

  const [autoPrune, setAutoPrune] = useState(() => {
    try {
      return window.localStorage.getItem(AUTOPRUNE_KEY) === "1";
    } catch {
      return false;
    }
  });

  // 切换「启动自动清理」：落 localStorage，并立刻手动跑一次让用户看到效果。
  const setAutoPrunePref = (v: boolean) => {
    setAutoPrune(v);
    try {
      window.localStorage.setItem(AUTOPRUNE_KEY, v ? "1" : "0");
    } catch {
      /* 隐私模式下写不了：本次会话内仍然生效 */
    }
  };
  const toggleAutoPrune = async () => {
    const next = !autoPrune;
    setAutoPrunePref(next);
    if (next) await pruneRecent();
  };

  // 恢复默认：提取影片=开、启动自动清理=关。
  const resetSettings = () => {
    setExtractVideoPref(true);
    setAutoPrunePref(false);
  };
  const [textCounts, setTextCounts] = useState<Record<string, number>>({});
  const [favs, setFavs] = useState<Set<string>>(new Set()); // event|gameID|itemID
  const [favItems, setFavItems] = useState<FavoriteDTO[]>([]); // 收藏区列表（含标题）
  const [favGames, setFavGames] = useState<Set<string>>(new Set()); // game 级收藏（游戏 id）
  const [favChars, setFavChars] = useState<Set<string>>(new Set()); // character|gameID|name
  const [favCharItems, setFavCharItems] = useState<FavoriteDTO[]>([]); // 收藏角色列表
  const [speakers, setSpeakers] = useState<SpeakerDTO[]>([]); // 说话人聚合（角色区）
  const [indexing, setIndexing] = useState<{
    title: string;
    done: number;
    total: number;
  } | null>(null);
  const allBooted = useRef(false); // StrictMode 双挂载只加载一次
  const [collapsed, setCollapsed] = useState(() => {
    try {
      return window.localStorage.getItem(SIDEBAR_KEY) === "1";
    } catch {
      return false;
    }
  });
  const covers = useRef(new Map<string, string>()); // game:id → dataURL 缓存

  // 启动链：游戏登记表 → 当前游戏的库（有清单读清单，没有就扫描建库）。
  const boot = async () => {
    try {
      const gs = await api.listGames();
      setGames(gs);
      void refreshFavs(); // 收藏状态/计数（失败不阻断主流程）
      if (autoPrune) void pruneRecent(false); // 启动自动清理 72h 前记录（仅 N>0 才提示）
      if (!gs.active) {
        setStatus({ state: "setup" });
        return;
      }
      await reload(false);
    } catch (e: any) {
      setStatus({ state: "error", message: e?.message ?? String(e) });
    }
  };

  const reload = async (force = false) => {
    setStatus({ state: "loading" });
    try {
      const l = force ? await api.rescan() : await api.getLibrary();
      setLib(l);
      setStatus({ state: "ready" });
      void refreshCatalog();
    } catch (e: any) {
      const msg = e?.message ?? String(e);
      setLib((prev) => prev ?? null);
      setStatus(
        msg.includes("还没有游戏库") || msg.includes("未设置游戏根目录")
          ? { state: "setup" }
          : { state: "error", message: msg },
      );
    }
  };

  // 中心库只读统计；失败时 Error 落到角标，不阻断回放。
  const refreshCatalog = async () => {
    try {
      setCatalog(await api.catalogStats());
    } catch (e: any) {
      setCatalog({
        games: 0,
        events: 0,
        text_lines: 0,
        error: e?.message ?? String(e),
      });
    }
  };

  // ---------- 「全部」页签数据 ----------

  // 串行索引一批游戏的台词（CatalogIndexText 幂等；单游戏失败不阻断其余）。
  const indexGames = async (list: GameDTO[]) => {
    for (let i = 0; i < list.length; i++) {
      const g = list[i];
      setIndexing({ title: g.title, done: i, total: list.length });
      try {
        await api.catalogIndexText(g.id);
      } catch (e: any) {
        setAllWarnings((w) => [
          ...w,
          `索引「${g.title}」台词失败：${e?.message ?? String(e)}`,
        ]);
      }
    }
    setIndexing(null);
    setTextIndexed(true); // 台词已抽取（角色区的「抽取台词」提示随之消失）
    try {
      const [tc, st] = await Promise.all([api.catalogTextCounts(), api.catalogStats()]);
      setTextCounts(tc);
      setCatalog(st);
    } catch {
      /* 角标刷新失败不影响已写入的索引 */
    }
  };

  // 收藏全量刷新（event 列表 + game 星标集 + character 星标集）；
  // 启动与「全部」页签进入时调。
  const refreshFavs = async () => {
    try {
      const [ev, gm, ch] = await Promise.all([
        api.listFavorites("event"),
        api.listFavorites("game"),
        api.listFavorites("character"),
      ]);
      setFavItems(ev);
      setFavs(new Set(ev.map((f) => `event|${f.game_id}|${f.item_id}`)));
      setFavGames(new Set(gm.map((f) => f.item_id)));
      setFavCharItems(ch);
      setFavChars(new Set(ch.map((f) => `character|${f.game_id}|${f.item_id}`)));
    } catch {
      /* 收藏读取失败不阻断主流程（星标/计数保持空，FR-12 见操作时警告） */
    }
  };

  // 进入「全部」：查询前由后端逐游戏同步 catalog；0 台词游戏按需抽台词。
  const loadAll = async () => {
    try {
      const gs = await api.listGames();
      setGames(gs);
      const [ev, rc, tc] = await Promise.all([
        api.catalogEvents("", 2000),
        api.catalogRecent(RECENT_LIMIT),
        api.catalogTextCounts(),
      ]);
      setAllEvents(ev.events);
      setAllWarnings(ev.warnings);
      setRecentItems(rc);
      setTextCounts(tc);
      await refreshFavs();
      const pending = gs.games.filter((g) => !tc[g.id]);
      if (pending.length > 0) await indexGames(pending);
      // 说话人依赖 text_lines：必须在索引之后拉（0 台词游戏刚抽完）。
      setSpeakers(await api.catalogSpeakers(""));
    } catch (e: any) {
      setAllWarnings([e?.message ?? String(e)]);
    }
  };

  // 收藏/取消收藏（event 级）：乐观更新，成功后重拉列表（收藏区要标题），
  // 失败回滚 + 警告（FR-12 不静默）。
  const toggleFav = async (gameID: string, _kind: string, id: string) => {
    const key = `event|${gameID}|${id}`;
    const had = favs.has(key);
    setFavs((prev) => {
      const next = new Set(prev);
      if (had) next.delete(key);
      else next.add(key);
      return next;
    });
    try {
      if (had) await api.removeFavorite("event", gameID, id);
      else await api.addFavorite("event", gameID, id, 0, "");
      await refreshFavs();
    } catch (e: any) {
      setFavs((prev) => {
        const next = new Set(prev);
        if (had) next.add(key);
        else next.delete(key);
        return next;
      });
      setAllWarnings((w) => [
        ...w,
        `${had ? "取消收藏" : "收藏"}失败：${e?.message ?? String(e)}`,
      ]);
    }
  };

  // 保存收藏的评分/备注（AddFavorite upsert；成功重拉，失败上抛让编辑态保持）。
  const saveFavMeta = async (gameID: string, itemID: string, rating: number, note: string) => {
    try {
      await api.addFavorite("event", gameID, itemID, rating, note);
      await refreshFavs();
    } catch (e: any) {
      setAllWarnings((w) => [
        ...w,
        `保存收藏信息失败：${e?.message ?? String(e)}`,
      ]);
      throw e;
    }
  };

  // 收藏/取消收藏游戏（game 级：item_id = game_id）。乐观更新 + 失败回滚。
  const toggleGameFav = async (gameID: string) => {
    const had = favGames.has(gameID);
    setFavGames((prev) => {
      const next = new Set(prev);
      if (had) next.delete(gameID);
      else next.add(gameID);
      return next;
    });
    try {
      if (had) await api.removeFavorite("game", gameID, gameID);
      else await api.addFavorite("game", gameID, gameID, 0, "");
    } catch (e: any) {
      setFavGames((prev) => {
        const next = new Set(prev);
        if (had) next.add(gameID);
        else next.delete(gameID);
        return next;
      });
      setAllWarnings((w) => [
        ...w,
        `${had ? "取消收藏游戏" : "收藏游戏"}失败：${e?.message ?? String(e)}`,
      ]);
    }
  };

  // 收藏/取消收藏角色（character 级：item_id = 说话人名，game_id 区分同名校色）。
  // 乐观更新 + 失败回滚；成功后重拉（收藏角色区要标题）。
  const toggleCharFav = async (gameID: string, name: string) => {
    const key = `character|${gameID}|${name}`;
    const had = favChars.has(key);
    setFavChars((prev) => {
      const next = new Set(prev);
      if (had) next.delete(key);
      else next.add(key);
      return next;
    });
    try {
      if (had) await api.removeFavorite("character", gameID, name);
      else await api.addFavorite("character", gameID, name, 0, "");
      await refreshFavs();
    } catch (e: any) {
      setFavChars((prev) => {
        const next = new Set(prev);
        if (had) next.add(key);
        else next.delete(key);
        return next;
      });
      setAllWarnings((w) => [
        ...w,
        `${had ? "取消收藏角色" : "收藏角色"}失败：${e?.message ?? String(e)}`,
      ]);
    }
  };

  // 保存角色收藏的评分/备注（AddFavorite upsert；成功重拉，失败上抛让编辑态保持）。
  const saveCharMeta = async (gameID: string, name: string, rating: number, note: string) => {
    try {
      await api.addFavorite("character", gameID, name, rating, note);
      await refreshFavs();
    } catch (e: any) {
      setAllWarnings((w) => [
        ...w,
        `保存角色收藏信息失败：${e?.message ?? String(e)}`,
      ]);
      throw e;
    }
  };

  // 清理 72h（3 天）前的最近播放：删完顺手刷新，删除条数进提示（不静默）。
  const pruneRecent = async (alwaysReport = true) => {
    setPruningRecent(true);
    try {
      const n = await api.catalogPruneRecent(PRUNE_DAYS);
      setRecentItems(await api.catalogRecent(RECENT_LIMIT));
      // 手点必报（0 条也算反馈）；启动自动清理只在真删了东西时报，避免每次启动刷屏。
      if (alwaysReport || n > 0) {
        setAllWarnings((w) => [
          ...w,
          alwaysReport
            ? n > 0
              ? `已清理 ${n} 条 ${PRUNE_DAYS} 天前的最近播放记录`
              : `没有 ${PRUNE_DAYS} 天前的最近播放记录可清理`
            : `启动自动清理：已清理 ${n} 条 ${PRUNE_DAYS} 天前的最近播放记录`,
        ]);
      }
    } catch (e: any) {
      setAllWarnings((w) => [
        ...w,
        `清理最近播放失败：${e?.message ?? String(e)}`,
      ]);
    } finally {
      setPruningRecent(false);
    }
  };

  // 切到「全部」时加载一次（StrictMode 双挂载用 ref 防重入）。
  useEffect(() => {
    if (tab !== "all" || allBooted.current) return;
    allBooted.current = true;
    void loadAll();
  }, [tab]);

  // 跨游戏打开：先切到该游戏（后端 SetActiveGame 重扫/读库），再进回放。
  const openFromGame = async (
    gameID: string,
    kind: string,
    id: string,
    title: string,
  ) => {
    if (gameID && gameID !== games?.active) {
      setBusy(true);
      try {
        const l = await api.setActiveGame(gameID);
        const gs = await api.listGames();
        setGames(gs);
        setLib(l);
        covers.current.clear();
      } catch (e: any) {
        setStatus({ state: "error", message: e?.message ?? String(e) });
        return;
      } finally {
        setBusy(false);
      }
    }
    onOpen(kind, id, title);
  };

  // 封面按游戏路由（跨游戏卡片墙）；缺失回 null，不阻断卡片。
  const coverFor = async (
    gameID: string,
    kind: string,
    id: string,
  ): Promise<string | null> => {
    const key = `${gameID}:${kind}:${id}`;
    const hit = covers.current.get(key);
    if (hit) return hit;
    try {
      const url = await api.getCoverForGame(gameID, kind, id);
      covers.current.set(key, url);
      return url;
    } catch {
      return null;
    }
  };

  useEffect(() => {
    void boot();
  }, []);

  // 左侧游戏栏折叠状态本地记忆；Ctrl+B 与 VS Code 一致。
  useEffect(() => {
    try {
      window.localStorage.setItem(SIDEBAR_KEY, collapsed ? "1" : "0");
    } catch {
      /* 无持久化不影响使用 */
    }
  }, [collapsed]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "b") {
        e.preventDefault();
        setCollapsed((v) => !v);
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  // 添加游戏（同一目录重复添加 = 切换并重扫，后端保证不重复登记）。
  const pickAndAdd = async () => {
    try {
      const dir = await api.pickDirectory();
      if (!dir) return;
      setBusy(true);
      setStatus({ state: "loading" });
      const l = await api.addGame(dir);
      const gs = await api.listGames();
      setGames(gs);
      covers.current.clear();
      setLib(l);
      setTab("cg");
      setStatus({ state: "ready" });
      void refreshCatalog();
    } catch (e: any) {
      setStatus({ state: "error", message: e?.message ?? String(e) });
    } finally {
      setBusy(false);
    }
  };

  // 解包导入：先预览（给用户看「会解出什么」），确认后再解。
  const previewUnpack = async (path: string) => {
    try {
      setBusy(true);
      const plan = await api.unpackPreview(path);
      setUnpackPath(path);
      setUnpackPlan(plan);
      setAskVideo(false);
      setForceReextract(false);
      setUnpackResult(null);
      setUnpackScheme("");
      setUnpackKey("0");
      if (plan.unsupported) {
        setAllWarnings((w) => [...w, `没法自动解包：${plan.reason}`]);
      }
    } catch (e: any) {
      setAllWarnings((w) => [...w, `解包预览失败：${e?.message ?? String(e)}`]);
    } finally {
      setBusy(false);
    }
  };

  // 阶段 4 手动退路：打开解包产物/源目录（失败进常驻横幅）。
  const openLocalPath = async (path: string) => {
    try {
      await api.openPath(path);
    } catch (e: any) {
      setAllWarnings((w) => [...w, `打开目录失败：${e?.message ?? String(e)}`]);
    }
  };

  // 选目录 → 预览。
  const pickAndUnpack = async () => {
    try {
      const dir = await api.pickDirectory();
      if (!dir) return;
      await previewUnpack(dir);
    } catch (e: any) {
      setAllWarnings((w) => [...w, `选择路径失败：${e?.message ?? String(e)}`]);
    }
  };

  // 确认解包 → 解出最小集合 → 登记入库 → 切到该游戏。
  // skipVideo 由调用方显式传入（有影片时先问一次，见 askVideo）。
  const runUnpack = async (skipVideo: boolean, force: boolean = false) => {
    if (!unpackPath) return;
    const scheme = unpackScheme.trim();
    let key = 0;
    if (scheme === "xor") {
      const raw = unpackKey.trim().toLowerCase();
      const n = raw.startsWith("0x") ? parseInt(raw.slice(2), 16) : parseInt(raw, 10);
      if (!Number.isInteger(n) || n < 0 || n > 255) {
        setAllWarnings((w) => [...w, "xor 密钥必须是 0-255（支持十进制或 0x 十六进制）"]);
        return;
      }
      key = n;
    }
    setUnpacking(true);
    setBusy(true);
    setAskVideo(false);
    try {
      const r: UnpackImportDTO = scheme
        ? await api.unpackImportDecrypt(unpackPath, scheme, key, skipVideo, force)
        : await api.unpackImport(unpackPath, skipVideo, force);
      setUnpackPlan(null);
      setUnpackPath("");
      if (!r.connected || !r.library) {
        // 解包成功但事件库还接不上：保留结果卡，给产物路径和手动退路。
        setUnpackResult(r);
        setAllWarnings((w) => [
          ...w,
          `已解包，但事件库还未接入：${r.reason ?? "未知原因"}`,
        ]);
        return;
      }
      const gs = await api.listGames();
      setGames(gs);
      covers.current.clear();
      setLib(r.library);
      setTab("cg");
      setStatus({ state: "ready" });
      setUnpackResult(null);
      setAllWarnings((w) => [
        ...w,
        `已导入：新增 ${r.files} 个文件${r.reused ? ` · 复用 ${r.reused} 个` : ""}（${(r.bytes / 1048576).toFixed(1)} MB）· ${r.events} 个事件 → ${r.dest}` +
          (r.missing.length ? ` · ${r.missing.length} 个素材没找到` : "") +
          (r.encrypted.length ? ` · ${r.encrypted.length} 个加密条目未解出` : ""),
      ]);
      void refreshCatalog();
    } catch (e: any) {
      setAllWarnings((w) => [...w, `解包失败：${e?.message ?? String(e)}`]);
    } finally {
      setUnpacking(false);
      setBusy(false);
    }
  };

  // 拖放：Go 侧 OnFileDrop 收到路径后 emit 「unpack:drop」，这里接住并预览。
  useEffect(() => {
    const rt = (window as any).runtime;
    if (!rt?.EventsOn) return;
    const off = rt.EventsOn("unpack:drop", (e: any) => {
      const p = e?.path ?? (Array.isArray(e) ? e[0] : "");
      if (p) void previewUnpack(String(p));
    });
    return () => {
      if (typeof off === "function") off();
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const switchGame = async (id: string) => {
    if (!id || id === games?.active || busy) return;
    setBusy(true);
    setBusyGame(id);
    try {
      const l = await api.setActiveGame(id);
      const gs = await api.listGames();
      setGames(gs);
      covers.current.clear();
      setLib(l);
      setTab("cg");
      setStatus({ state: "ready" });
      void refreshCatalog();
    } catch (e: any) {
      setStatus({ state: "error", message: e?.message ?? String(e) });
    } finally {
      setBusy(false);
      setBusyGame("");
    }
  };

  const cards: CardItem[] = useMemo(() => {
    if (!lib) return [];
    const query = q.trim().toLowerCase();
    const hit = (s: string) => !query || s.toLowerCase().includes(query);
    if (tab === "cg") {
      return (lib.cgs ?? [])
        .filter((c) => hit(c.title) || hit(c.id))
        .map((c) => ({
          kind: "cg" as const,
          id: c.id,
          title: c.title,
          // 旧库清单 cgreplay-lib.json 里 pages 可能为 null（CLI 旧格式），防御
          note: (c.pages ?? []).length > 1 ? `×${(c.pages ?? []).length}页` : "",
          ok: true,
        }));
    }
    if (tab === "scene") {
      return (lib.scenes ?? [])
        .filter((s) => hit(s.title))
        .map((s) => ({ kind: "scene" as const, id: s.id, title: s.title, note: "", ok: true as const }));
    }
    return (lib.chapters ?? [])
      .filter((ch) => hit(ch.title) || hit(ch.scene))
      .map((ch) => ({
        kind: "chapter" as const,
        id: ch.id,
        title: ch.title,
        note: `#${ch.index}`,
        ok: true,
      }));
  }, [lib, tab, q]);

  if (status.state === "setup") {
    return (
      <div className="flex min-h-screen items-center justify-center">
        <div className="flex w-[460px] flex-col items-center gap-4 rounded-lg border border-border bg-card px-10 py-10 text-center">
          <FolderOpen className="h-9 w-9 text-primary" />
          <h1 className="text-lg font-semibold">先选游戏解包目录</h1>
          <p className="text-sm leading-relaxed text-muted-foreground">
            选「包含 raw\data\scenario 的那一层」。选择后会自动建立事件库。
            之后可随时从左侧游戏栏底部的「添加游戏」登记更多游戏，配置与 CLI 版
            cgreplay.exe 共用同一份 cgreplay.conf。
          </p>
          <Button onClick={pickAndAdd} disabled={busy} className="mt-2">
            {busy ? "建立中…" : "选择目录"}
          </Button>
        </div>
      </div>
    );
  }

  if (status.state === "loading" || (busy && !lib)) {
    return (
      <div className="flex min-h-screen items-center justify-center text-muted-foreground">
        正在建立事件库…
      </div>
    );
  }

  if (status.state === "error" && !lib) {
    return (
      <div className="flex min-h-screen items-center justify-center">
        <div className="w-[520px] rounded-lg border border-red-900 bg-card px-8 py-7 text-center">
          <h1 className="text-base font-semibold text-red-400">出错了</h1>
          <p className="mt-3 break-all text-sm text-muted-foreground">{status.message}</p>
          <div className="mt-5 flex justify-center gap-3">
            <Button size="sm" variant="outline" onClick={pickAndAdd}>换个目录</Button>
            <Button size="sm" variant="outline" onClick={() => void reload(true)}>重试</Button>
          </div>
        </div>
      </div>
    );
  }

  const banner =
    status.state === "error" && lib ? status.message : null;
  const gameList = games?.games ?? [];
  const activeGame = gameList.find((g) => g.id === games?.active);
  const activeTitle = activeGame?.title ?? lib?.game ?? "CG 演出回放";

  return (
    <div className="flex h-screen overflow-hidden">
      <GameSidebar
        games={gameList}
        active={games?.active ?? ""}
        busy={busy}
        busyGame={busyGame}
        collapsed={collapsed}
        catalog={catalog}
        favGames={favGames}
        favEventCount={favItems.length}
        onToggle={() => setCollapsed((v) => !v)}
        onSelect={(id) => void switchGame(id)}
        onAdd={() => void pickAndAdd()}
        onUnpack={pickAndUnpack}
        onToggleGameFav={(id) => void toggleGameFav(id)}
      />

      <div className="flex min-w-0 flex-1 flex-col">
        {/* 顶栏：当前游戏 + 页签 + 搜索 + 刷新 */}
        <header className="flex h-14 shrink-0 items-center gap-2 border-b border-border bg-background/95 px-4 backdrop-blur lg:gap-3">
          <div className="hidden min-w-0 max-w-[180px] shrink-0 lg:block">
            <div className="flex items-center gap-2">
              <h1 className="truncate text-sm font-semibold">{activeTitle}</h1>
              {gameList.length > 1 && (
                <Badge variant="secondary" className="shrink-0 px-1.5 text-[10px]">
                  {gameList.length} 库
                </Badge>
              )}
            </div>
            <p className="truncate text-[10px] leading-tight text-muted-foreground" title={lib?.root}>
              {lib?.root ?? "从左侧选择游戏库"}
            </p>
          </div>
          <nav className="flex shrink-0 gap-1">
            {(
              [
                ["all", `全部 ${allEvents ? allEvents.length : "·"}`],
                ["cg", `CG鉴赏 ${lib?.cgs?.length ?? 0}`],
                ["scene", `主线场景 ${lib?.scenes?.length ?? 0}`],
                ["chapter", `演出段落 ${lib?.chapters?.length ?? 0}`],
              ] as [Tab, string][]
            ).map(([k, label]) => (
              <button
                key={k}
                onClick={() => setTab(k)}
                className={
                  "rounded-md px-2.5 py-1.5 text-sm transition-colors " +
                  (tab === k
                    ? "bg-secondary font-medium text-foreground"
                    : "text-muted-foreground hover:bg-accent hover:text-foreground")
                }
              >
                {label}
              </button>
            ))}
          </nav>
          <div className="flex-1" />
          <div className="relative shrink-0">
            <Search className="absolute left-2.5 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
            <input
              value={q}
              onChange={(e) => setQ(e.target.value)}
              placeholder={tab === "all" ? "按游戏名 / 事件标题筛选…" : "搜索 cg17 / 标题…"}
              className="h-9 w-32 rounded-md border border-input bg-input/40 pl-8 pr-3 text-sm outline-none placeholder:text-muted-foreground focus:ring-1 focus:ring-ring lg:w-44 xl:w-60"
            />
          </div>
          <Button
            variant="outline"
            size="icon"
            className="shrink-0"
            title="重扫当前游戏"
            onClick={() => void reload(true)}
          >
            <RefreshCw className="h-4 w-4" />
          </Button>
          <Button
            variant="outline"
            size="icon"
            className="shrink-0"
            title="设置"
            onClick={() => setSettingsOpen(true)}
          >
            <Settings className="h-4 w-4" />
          </Button>
        </header>

        {/* 错误横幅区：任何异常都常驻可见（对齐 FR-12） */}
        {banner && (
          <div className="shrink-0 bg-destructive px-5 py-1.5 text-sm">{banner}</div>
        )}
        {/* 非致命警告：CG/场景页也要可见（「全部」页有自己的警告区，避免重复） */}
        {tab !== "all" && allWarnings.length > 0 && (
          <div className="max-h-24 shrink-0 overflow-y-auto border-b border-amber-900/60 bg-amber-950/30 px-5 py-1.5 text-xs text-amber-200">
            {allWarnings.map((w, i) => (
              <div key={i} className="flex gap-2">
                <span aria-hidden>•</span>
                <span className="min-w-0 break-words">{w}</span>
              </div>
            ))}
          </div>
        )}

        {/* 解包预览（拖进来或选目录后：先看会解出什么，再确认） */}
        {unpackPlan && (
          <section className="rounded-md border border-primary/40 bg-primary/5 px-4 py-3">
            <div className="flex items-start gap-3">
              <PackageOpen className="mt-0.5 h-4 w-4 shrink-0 text-primary" />
              <div className="min-w-0 flex-1 space-y-1">
                <div className="text-sm font-medium">
                  {unpackPlan.unsupported ? "这个路径没法自动解包" : "可以自动解包"}
                </div>
                <div className="truncate text-[11px] text-muted-foreground" title={unpackPath}>
                  {unpackPath}
                </div>
                {!unpackPlan.unsupported && (
                  <div className="text-[11px] text-muted-foreground">
                    引擎 {unpackPlan.engine_id || "?"} · 剧本 {unpackPlan.scenario_files} 个 ·
                    预计 {unpackPlan.labels} 个事件 · 引用素材 {unpackPlan.refs} 个
                    {(unpackPlan.est_bytes ?? 0) > 0 &&
                      ` · 预计占盘 ${(unpackPlan.est_bytes / 1048576).toFixed(0)} MB`}
                    {unpackPlan.encrypted > 0 &&
                      ` · ${unpackPlan.encrypted} 个条目带 adlr 校验和（乱码时可选解密方案）`}
                  </div>
                )}
                {unpackPlan.kind === "archive" && unpackPlan.engine_id === "kirikiri" && (
                  <div className="flex flex-wrap items-center gap-2 pt-1 text-[11px]">
                    <span className="text-muted-foreground">解密方案</span>
                    <select
                      value={unpackScheme}
                      onChange={(e) => setUnpackScheme(e.target.value)}
                      disabled={unpacking}
                      aria-label="XP3 解密方案"
                      className="h-7 rounded-md border border-input bg-input/40 px-2 text-[11px] outline-none focus:ring-1 focus:ring-ring disabled:opacity-50"
                    >
                      <option value="">不解密（推荐先试）</option>
                      <option value="hash">hash（adlr 派生）</option>
                      <option value="natsupochi">natsupochi</option>
                      <option value="poringsoft">poringsoft</option>
                      <option value="sourire">sourire</option>
                      <option value="haikuo">haikuo</option>
                      <option value="xor">xor（固定密钥）</option>
                    </select>
                    {unpackScheme === "xor" && (
                      <input
                        value={unpackKey}
                        onChange={(e) => setUnpackKey(e.target.value)}
                        disabled={unpacking}
                        aria-label="xor 固定密钥"
                        placeholder="0-255 或 0x5A"
                        className="h-7 w-28 rounded-md border border-input bg-input/40 px-2 text-[11px] outline-none placeholder:text-muted-foreground focus:ring-1 focus:ring-ring disabled:opacity-50"
                      />
                    )}
                    <span className="text-muted-foreground">先试不解密；解出乱码再换方案</span>
                  </div>
                )}
                {!unpackPlan.unsupported && (unpackPlan.imported_videos ?? 0) > 0 && (
                  <div
                    className="pt-1 text-[11px]"
                    title="以磁盘上的实际文件为准；重解不会删除已有影片，只补缺的。"
                  >
                    {(unpackPlan.imported_videos ?? 0) < unpackPlan.videos ? (
                      <span className="text-amber-300">
                        已有 {unpackPlan.imported_videos} / {unpackPlan.videos} 个影片 ·
                        重解会补上差的 {unpackPlan.videos - (unpackPlan.imported_videos ?? 0)} 个
                      </span>
                    ) : (
                      <span className="text-emerald-400">
                        影片已全部提取（{unpackPlan.imported_videos} 个）
                      </span>
                    )}
                  </div>
                )}
                {!unpackPlan.unsupported &&
                  unpackPlan.videos > 0 &&
                  (unpackPlan.imported_videos ?? 0) === 0 && (
                  <div className="pt-1">
                    <label
                      className="flex cursor-pointer items-center gap-2 text-[11px] text-muted-foreground"
                      title="关掉后影片事件不能播；已解包的游戏需重解才能补上。之后可在右上角 ⚙ 设置里改。"
                    >
                      <input
                        type="checkbox"
                        checked={extractVideo}
                        onChange={(e) => setExtractVideoPref(e.target.checked)}
                        disabled={unpacking}
                        className="h-3.5 w-3.5 accent-primary"
                      />
                      <span>
                        提取 OP/ED 影片（共 {unpackPlan.videos} 个 ·{" "}
                        {(unpackPlan.video_bytes / 1048576).toFixed(0)} MB）
                      </span>
                    </label>
                  </div>
                )}
                {askVideo && !unpackPlan.unsupported && (
                  <div className="mt-1 rounded-md border border-primary/40 bg-primary/5 px-3 py-2">
                    <div className="text-[11px] text-foreground">
                      有 {unpackPlan.videos} 个影片（{(unpackPlan.video_bytes / 1048576).toFixed(0)} MB），一起提取？
                    </div>
                    <div className="mt-2 flex flex-wrap gap-2">
                      <Button
                        size="sm"
                        onClick={() => {
                          setExtractVideoPref(true);
                          void runUnpack(false, forceReextract);
                        }}
                        disabled={unpacking || busy}
                      >
                        一起提取（推荐）
                      </Button>
                      <Button
                        size="sm"
                        variant="outline"
                        onClick={() => {
                          setExtractVideoPref(false);
                          void runUnpack(true, forceReextract);
                        }}
                        disabled={unpacking || busy}
                      >
                        跳过影片
                      </Button>
                      <Button size="sm" variant="ghost" onClick={() => setAskVideo(false)} disabled={unpacking}>
                        再想想
                      </Button>
                    </div>
                    <div className="mt-1.5 text-[10px] text-muted-foreground">选择会记住，可在 ⚙ 设置里改</div>
                  </div>
                )}
                {!unpackPlan.unsupported && unpackPlan.imported && (
                  <label
                    className="flex cursor-pointer items-center gap-2 pt-1 text-[11px] text-muted-foreground"
                    title="默认只补缺失的文件；如果换了补丁但画面没变，勾这个全部重写一遍。"
                  >
                    <input
                      type="checkbox"
                      checked={forceReextract}
                      onChange={(e) => setForceReextract(e.target.checked)}
                      disabled={unpacking}
                      className="h-3.5 w-3.5 accent-primary"
                    />
                    <span>强制重解（全部重写；默认只补缺失）</span>
                  </label>
                )}
                {unpackPlan.unsupported && (
                  <div className="text-[11px] text-amber-300">{unpackPlan.reason}</div>
                )}
                {unpackPlan.notes.map((n, i) => (
                  <div key={i} className="text-[11px] text-amber-300">
                    {n}
                  </div>
                ))}
              </div>
              <div className="flex shrink-0 gap-2">
                <Button variant="ghost" size="sm" onClick={() => void openLocalPath(unpackPath)}>
                  打开位置
                </Button>
                <Button variant="ghost" size="sm" onClick={() => setUnpackPlan(null)} disabled={unpacking}>
                  取消
                </Button>
                {!unpackPlan.unsupported && (
                  <Button
                    size="sm"
                    onClick={() => {
                      // 默认提取 → 有影片就先问一次；用户已取消勾选则直接按跳过走。
                      const already = (unpackPlan.imported_videos ?? 0) > 0;
                      if (extractVideo && !already && unpackPlan.videos > 0 && !askVideo) {
                        setAskVideo(true);
                        return;
                      }
                      void runUnpack(!extractVideo, forceReextract);
                    }}
                    disabled={unpacking || busy}
                  >
                    {unpacking ? "解包中…" : "开始解包"}
                  </Button>
                )}
              </div>
            </div>
          </section>
        )}

        {/* 已解包但事件库未接入：产物保留，给打开目录/手动退路（阶段 4） */}
        {unpackResult && (
          <section className="border-y border-amber-900/60 bg-amber-950/20 px-4 py-3">
            <div className="flex items-start gap-3">
              <PackageOpen className="mt-0.5 h-4 w-4 shrink-0 text-amber-300" />
              <div className="min-w-0 flex-1 space-y-1">
                <div className="text-sm font-medium text-amber-200">
                  已解包，但还不能进事件库
                </div>
                <div className="text-[11px] text-muted-foreground">
                  引擎 {unpackResult.engine_id || "?"} · 解出 {unpackResult.files} 个文件（
                  {(unpackResult.bytes / 1048576).toFixed(1)} MB）· 识别到 {unpackResult.events} 个事件
                  {unpackResult.missing.length ? ` · ${unpackResult.missing.length} 个素材没找到` : ""}
                  {unpackResult.encrypted.length ? ` · ${unpackResult.encrypted.length} 个加密条目未解出` : ""}
                </div>
                <div className="truncate text-[11px] text-muted-foreground" title={unpackResult.dest}>
                  {unpackResult.dest}
                </div>
                {unpackResult.reason && (
                  <div className="text-[11px] text-amber-300">{unpackResult.reason}</div>
                )}
                {unpackResult.hint && (
                  <div className="text-[11px] leading-relaxed text-muted-foreground">
                    {unpackResult.hint}
                  </div>
                )}
              </div>
              <div className="flex shrink-0 items-center gap-2">
                <Button variant="outline" size="sm" onClick={() => void openLocalPath(unpackResult.dest)}>
                  <FolderOpen className="mr-1.5 h-3.5 w-3.5" />
                  打开目录
                </Button>
                <Button variant="ghost" size="sm" onClick={() => setUnpackResult(null)}>
                  关闭
                </Button>
              </div>
            </div>
          </section>
        )}

        {/* 「全部」页签：跨游戏卡片墙 / 台词检索 / 最近播放 */}
        {tab === "all" ? (
          <AllGames
            events={allEvents ?? []}
            loaded={allEvents !== null}
            warnings={allWarnings}
            indexing={indexing}
            recent={recentItems}
            filter={q}
            pruningRecent={pruningRecent}
            autoPrune={autoPrune}
            onPruneRecent={() => pruneRecent(true)}
            onToggleAutoPrune={() => void toggleAutoPrune()}
            favItems={favItems}
            favs={favs}
            speakers={speakers}
            favChars={favChars}
            favCharItems={favCharItems}
            textIndexed={textIndexed}
            textCounts={textCounts}
            busy={busy}
            onOpen={openFromGame}
            onCover={coverFor}
            onToggleFav={toggleFav}
            onSaveFavMeta={saveFavMeta}
            onToggleCharFav={toggleCharFav}
            onSaveCharMeta={saveCharMeta}
            onIndexAll={() => {
              const pending = gameList.filter((g) => !textCounts[g.id]);
              if (pending.length > 0) void indexGames(pending);
              else void loadAll();
            }}
          />
        ) : (
          /* 卡片墙 */
          <main className="grid flex-1 auto-rows-max content-start gap-4 overflow-y-auto p-5"
            style={{ gridTemplateColumns: "repeat(auto-fill, minmax(220px, 1fr))" }}>
            {cards.map((c) => (
              <EventCard
                key={`${lib?.root ?? ""}:${c.kind}:${c.id}`}
                card={c}
                gameID={games?.active ?? ""}
                lookup={coverFor}
                onClick={() => onOpen(c.kind, c.id, c.title)}
              />
            ))}
            {cards.length === 0 && (
              <div className="col-span-full py-24 text-center text-sm text-muted-foreground">
                没有匹配的事件
              </div>
            )}
          </main>
        )}
      </div>

      <SettingsDialog
        open={settingsOpen}
        onClose={() => setSettingsOpen(false)}
        extractVideo={extractVideo}
        onChangeExtractVideo={setExtractVideoPref}
        autoPrune={autoPrune}
        onToggleAutoPrune={() => void toggleAutoPrune()}
        onReset={resetSettings}
      />
    </div>
  );
}

// ---------- 左侧游戏栏（VS Code 侧栏风格） ----------

function GameSidebar({
  games,
  active,
  busy,
  busyGame,
  collapsed,
  catalog,
  favGames,
  favEventCount,
  onToggle,
  onSelect,
  onAdd,
  onUnpack,
  onToggleGameFav,
}: {
  games: GameDTO[];
  active: string;
  busy: boolean;
  busyGame: string;
  collapsed: boolean;
  catalog: CatalogStatsDTO | null;
  favGames: Set<string>;
  favEventCount: number;
  onToggle: () => void;
  onSelect: (id: string) => void;
  onAdd: () => void;
  onUnpack: () => void;
  onToggleGameFav: (id: string) => void;
}) {
  const [filter, setFilter] = useState("");
  const q = filter.trim().toLowerCase();
  const shown = games.filter(
    (g) =>
      !q ||
      g.title.toLowerCase().includes(q) ||
      g.root.toLowerCase().includes(q) ||
      g.id.toLowerCase().includes(q),
  );
  const shownFav = shown.filter((g) => favGames.has(g.id));
  const shownRest = shown.filter((g) => !favGames.has(g.id));

  // 一行游戏：主点击区（选择/切换）+ 行尾星标（game 级收藏）。
  // 星标是主 button 的兄弟节点——button 嵌 button 不合法。
  const renderRow = (g: GameDTO) => {
    const isActive = g.id === active;
    const pending = busyGame === g.id;
    const fav = favGames.has(g.id);
    return (
      <div
        key={g.id}
        className={cn(
          "group relative flex items-center gap-1 rounded-md pr-1 transition-colors",
          isActive
            ? "bg-secondary text-foreground"
            : "text-muted-foreground hover:bg-accent/60 hover:text-foreground",
          busy && "cursor-not-allowed opacity-60",
        )}
      >
        {isActive && (
          <span className="absolute left-0 top-1/2 h-5 w-0.5 -translate-y-1/2 rounded-full bg-primary" />
        )}
        <button
          onClick={() => onSelect(g.id)}
          disabled={busy}
          aria-current={isActive ? "page" : undefined}
          title={`${g.title}\n${g.root}`}
          className="flex min-w-0 flex-1 items-center gap-2.5 rounded-md px-2 py-2 text-left"
        >
          <span
            className={cn(
              "flex h-8 w-8 shrink-0 items-center justify-center rounded-md border",
              isActive
                ? "border-primary/40 bg-primary/15 text-primary"
                : "border-border bg-background/40",
            )}
          >
            <Gamepad2 className="h-4 w-4" />
          </span>
          <span className="min-w-0 flex-1">
            <span className="block truncate text-[13px] font-medium">{g.title}</span>
            <span className="block truncate text-[10px] text-muted-foreground">
              {shortRoot(g.root)}
            </span>
          </span>
        </button>
        <button
          onClick={() => onToggleGameFav(g.id)}
          disabled={busy}
          title={fav ? "取消收藏游戏" : "收藏游戏"}
          className={cn(
            "flex h-7 w-7 shrink-0 items-center justify-center rounded-md transition-opacity",
            fav
              ? "text-amber-400 opacity-100"
              : "text-muted-foreground/60 opacity-0 hover:text-amber-400 focus-visible:opacity-100 group-hover:opacity-100",
            busy && "cursor-not-allowed opacity-50",
          )}
        >
          <Star className={cn("h-3.5 w-3.5", fav && "fill-current")} />
        </button>
        {pending ? (
          <Loader2 className="h-3.5 w-3.5 shrink-0 animate-spin" />
        ) : isActive ? (
          <span className="h-1.5 w-1.5 shrink-0 rounded-full bg-primary" />
        ) : null}
      </div>
    );
  };

  if (collapsed) {
    return (
      <aside className="flex h-full w-16 shrink-0 flex-col border-r border-border bg-card/30">
        <div className="flex h-14 shrink-0 items-center justify-center border-b border-border">
          <div className="flex h-8 w-8 items-center justify-center rounded-md bg-primary/15 text-primary">
            <Ship className="h-4 w-4" />
          </div>
        </div>
        <div className="min-h-0 flex-1 space-y-1 overflow-y-auto px-2 py-3">
          {games.map((g) => {
            const isActive = g.id === active;
            const pending = busyGame === g.id;
            return (
              <button
                key={g.id}
                onClick={() => onSelect(g.id)}
                disabled={busy}
                aria-current={isActive ? "page" : undefined}
                title={`${g.title}\n${g.root}`}
                className={cn(
                  "mx-auto flex h-9 w-9 items-center justify-center rounded-md border text-[11px] font-semibold transition-colors",
                  isActive
                    ? "border-primary/50 bg-primary/15 text-primary"
                    : "border-transparent text-muted-foreground hover:bg-accent hover:text-foreground",
                  busy && "cursor-not-allowed opacity-50",
                )}
              >
                {pending ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : gameInitials(g.title)}
              </button>
            );
          })}
        </div>
        <div className="space-y-1 border-t border-border p-2">
          <div
            title={catalogTitle(catalog)}
            className="mx-auto flex h-8 w-9 items-center justify-center"
          >
            <Database
              className={cn(
                "h-4 w-4",
                catalog?.error ? "text-amber-400" : "text-muted-foreground/70",
              )}
            />
          </div>
          <button
            onClick={onAdd}
            disabled={busy}
            title="添加游戏"
            className="mx-auto flex h-9 w-9 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-accent hover:text-foreground disabled:cursor-not-allowed disabled:opacity-50"
          >
            <Plus className="h-4 w-4" />
          </button>
          <button
            onClick={onToggle}
            title="展开游戏栏 (Ctrl+B)"
            className="mx-auto flex h-9 w-9 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
          >
            <PanelLeftOpen className="h-4 w-4" />
          </button>
        </div>
      </aside>
    );
  }

  return (
    <aside className="flex h-full w-[240px] shrink-0 flex-col border-r border-border bg-card/30">
      <div className="flex h-14 shrink-0 items-center gap-2 border-b border-border px-3">
        <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-md bg-primary/15 text-primary">
          <Ship className="h-4 w-4" />
        </div>
        <div className="min-w-0 flex-1 leading-tight">
          <div className="truncate text-[13px] font-semibold">CG 演出回放</div>
          <div className="truncate text-[10px] text-muted-foreground">多游戏库</div>
        </div>
        <Button
          variant="ghost"
          size="icon"
          className="h-7 w-7 shrink-0 text-muted-foreground"
          title="收起游戏栏 (Ctrl+B)"
          onClick={onToggle}
        >
          <PanelLeftClose className="h-4 w-4" />
        </Button>
      </div>

      <div className="flex items-center justify-between px-3 pb-1.5 pt-4">
        <span className="text-[10px] font-semibold uppercase tracking-[0.16em] text-muted-foreground">
          游戏库
        </span>
        <Badge variant="secondary" className="px-1.5 text-[10px]">
          {games.length}
        </Badge>
      </div>

      {games.length > 4 && (
        <div className="px-2 pb-2">
          <div className="relative">
            <Search className="absolute left-2 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" />
            <input
              value={filter}
              onChange={(e) => setFilter(e.target.value)}
              placeholder="筛选游戏…"
              className="h-8 w-full rounded-md border border-input bg-input/40 pl-7 pr-2 text-xs outline-none placeholder:text-muted-foreground focus:ring-1 focus:ring-ring"
            />
          </div>
        </div>
      )}

      <div className="min-h-0 flex-1 space-y-0.5 overflow-y-auto px-2 pb-2">
        {/* 收藏的游戏置顶（game 级收藏；星标在行尾，hover 显现/已收藏常驻） */}
        {shownFav.length > 0 && (
          <>
            <div className="flex items-center justify-between px-1 pb-1 pt-1">
              <span className="text-[10px] font-semibold uppercase tracking-[0.16em] text-amber-400/80">
                收藏的游戏
              </span>
              <Badge variant="secondary" className="px-1.5 text-[10px]">
                {shownFav.length}
              </Badge>
            </div>
            {shownFav.map(renderRow)}
          </>
        )}
        <div className="flex items-center justify-between px-1 pb-1 pt-1">
          <span className="text-[10px] font-semibold uppercase tracking-[0.16em] text-muted-foreground">
            游戏库
          </span>
          <Badge variant="secondary" className="px-1.5 text-[10px]">
            {games.length}
          </Badge>
        </div>
        {shownRest.map(renderRow)}
        {shown.length === 0 && (
          <div className="px-2 py-6 text-center text-xs text-muted-foreground">
            没有匹配的游戏
          </div>
        )}
      </div>

      <div className="space-y-1 border-t border-border px-3 py-2">
        <div className="flex items-center gap-1.5 text-[10px]" title={catalogTitle(catalog)}>
          <Database
            className={cn(
              "h-3.5 w-3.5 shrink-0",
              catalog?.error ? "text-amber-400" : "text-primary/70",
            )}
          />
          {catalog?.error ? (
            <span className="truncate text-amber-400">索引失败</span>
          ) : catalog ? (
            <span className="truncate text-muted-foreground">
              索引 {catalog.events} 事件 · {catalog.games} 库
            </span>
          ) : (
            <span className="truncate text-muted-foreground">索引检查中…</span>
          )}
        </div>
        <div
          className="flex items-center gap-1.5 text-[10px]"
          title={`收藏 ${favGames.size} 个游戏 / ${favEventCount} 个事件\n游戏星标在行尾；事件星标在「全部」页签卡片与回放页。`}
        >
          <Star className="h-3.5 w-3.5 shrink-0 text-amber-400/80" />
          <span className="truncate text-muted-foreground">
            收藏 {favGames.size} 游戏 · {favEventCount} 事件
          </span>
        </div>
      </div>

      <div className="space-y-1 border-t border-border p-2">
        <Button
          variant="ghost"
          className="h-9 w-full justify-start gap-2 px-2 text-xs text-muted-foreground hover:text-foreground"
          onClick={onAdd}
          disabled={busy}
        >
          {busy ? <Loader2 className="h-4 w-4 animate-spin" /> : <Plus className="h-4 w-4" />}
          {busy ? "正在登记…" : "添加游戏"}
        </Button>
        <Button
          variant="ghost"
          className="h-9 w-full justify-start gap-2 px-2 text-xs text-muted-foreground hover:text-foreground"
          onClick={onUnpack}
          disabled={busy}
          title="把 galgame 拖进窗口，或点这里选目录：自动解包并剪成「只够重演 CG 事件」的最小集合"
        >
          <PackageOpen className="h-4 w-4" />
          导入游戏（自动解包）
        </Button>
      </div>
    </aside>
  );
}

/** 索引角标 tooltip：失败原因常驻可查，不静默。 */
function catalogTitle(c: CatalogStatsDTO | null): string {
  if (!c) return "中心库索引检查中…";
  if (c.error) return `中心库索引失败：${c.error}`;
  return `本地 SQLite 索引\n${c.games} 个游戏 / ${c.events} 个事件 / ${c.text_lines} 条台词\n只存可检索元数据，回放仍读原始素材。`;
}

/** 折叠态图标：中文取首字，拉丁字母取前两位。 */
function gameInitials(title: string): string {
  const t = (title ?? "").trim();
  if (!t) return "?";
  const chars = Array.from(t);
  return /^[A-Za-z0-9]/.test(chars[0]) ? chars.slice(0, 2).join("").toUpperCase() : chars[0];
}

/** 侧栏副标题只显示盘符 + 父目录；标题本身通常就是游戏目录名，完整路径放 title。 */
function shortRoot(root: string): string {
  const parts = root.split(/[\\/]+/).filter(Boolean);
  if (parts.length === 0) return root;
  const tail = parts[parts.length - 1];
  let parent = parts[parts.length - 2];
  if (parent && /^[0-9a-f]{12,}$/i.test(parent) && parts.length >= 3) {
    parent = parts[parts.length - 3];
  }
  const drive = /^[A-Za-z]:$/.test(parts[0]) ? `${parts[0]}\\…` : "…";
  return parent ? `${drive}\\${parent}` : `${drive}\\${tail}`;
}

// ---------- 「全部」页签：跨游戏卡片墙 / 台词检索 / 最近播放 ----------

/** catalog 事件 → 卡片视图（kind 收窄成 CardItem 联合，未知 kind 走 ok:false）。 */
function cardFor(e: CatalogEventDTO): CardItem {
  switch (e.kind) {
    case "cg":
      return { kind: "cg", id: e.id, title: e.title, note: e.pages > 1 ? `×${e.pages}页` : "", ok: true };
    case "scene":
      return { kind: "scene", id: e.id, title: e.title, note: "", ok: true };
    case "chapter":
      return { kind: "chapter", id: e.id, title: e.title, note: "", ok: true };
    default:
      return { kind: e.kind, id: e.id, title: e.title, note: "", ok: false };
  }
}

function AllGames({
  events,
  loaded,
  warnings,
  indexing,
  recent,
  pruningRecent,
  autoPrune,
  onPruneRecent,
  onToggleAutoPrune,
  favItems,
  favs,
  speakers,
  favChars,
  favCharItems,
  filter,
  textIndexed,
  textCounts,
  busy,
  onOpen,
  onCover,
  onToggleFav,
  onSaveFavMeta,
  onToggleCharFav,
  onSaveCharMeta,
  onIndexAll,
}: {
  events: CatalogEventDTO[];
  loaded: boolean;
  warnings: string[];
  indexing: { title: string; done: number; total: number } | null;
  recent: CatalogRecentDTO[];
  pruningRecent: boolean;
  autoPrune: boolean;
  onPruneRecent: () => void;
  onToggleAutoPrune: () => void;
  favItems: FavoriteDTO[];
  favs: Set<string>;
  speakers: SpeakerDTO[];
  favChars: Set<string>;
  favCharItems: FavoriteDTO[];
  filter: string;
  textIndexed: boolean;
  textCounts: Record<string, number>;
  busy: boolean;
  onOpen: (gameID: string, kind: string, id: string, title: string) => void;
  onCover: (gameID: string, kind: string, id: string) => Promise<string | null>;
  onToggleFav: (gameID: string, kind: string, id: string) => void;
  onSaveFavMeta: (gameID: string, itemID: string, rating: number, note: string) => Promise<void>;
  onToggleCharFav: (gameID: string, name: string) => void;
  onSaveCharMeta: (gameID: string, name: string, rating: number, note: string) => Promise<void>;
  onIndexAll: () => void;
}) {
  // 按游戏分组（后端按游戏标题排序，保持出现序即分组序）。
  // filter = 顶部搜索框：按**游戏名 / 事件标题**过滤（台词检索已按用户要求撤除）。
  const groups = useMemo(() => {
    const query = filter.trim().toLowerCase();
    const hit = (s: string) => !query || s.toLowerCase().includes(query);
    const out: { game: CatalogEventDTO; items: CatalogEventDTO[] }[] = [];
    const idx = new Map<string, number>();
    for (const e of events) {
      // 命中条件含游戏名：按游戏名搜就该保留这款游戏的全部卡片。
      if (query && !hit(e.game_title) && !hit(e.title) && !hit(e.id)) continue;
      let gi = idx.get(e.game_id);
      if (gi === undefined) {
        gi = out.length;
        idx.set(e.game_id, gi);
        out.push({ game: e, items: [] });
      }
      out[gi].items.push(e);
    }
    return out;
  }, [events, filter]);

  // (game_id, item_id) → kind：收藏行打开事件时定位类型
  //（favorites 不存 kind；item_id 在各作 library 内实际唯一）。
  const kindOf = (gameID: string, id: string): string | null =>
    events.find((e) => e.game_id === gameID && e.id === id)?.kind ?? null;

  // 说话人按游戏分组（后端已按条数降序，组内保持）。
  const speakerGroups = useMemo(() => {
    const out: { game_id: string; game_title: string; items: SpeakerDTO[] }[] = [];
    const idx = new Map<string, number>();
    for (const s of speakers) {
      let gi = idx.get(s.game_id);
      if (gi === undefined) {
        gi = out.length;
        idx.set(s.game_id, gi);
        out.push({ game_id: s.game_id, game_title: s.game_title, items: [] });
      }
      out[gi].items.push(s);
    }
    return out;
  }, [speakers]);

  // 角色收藏的评分/备注（key = gameID:name）；chips 显示 ★N、编辑器回显。
  const charMeta = useMemo(() => {
    const m = new Map<string, FavoriteDTO>();
    for (const f of favCharItems) m.set(`${f.game_id}:${f.item_id}`, f);
    return m;
  }, [favCharItems]);

  // 最近播放按本地日历日分组（今天/昨天/M月D日）；recent 已按时间降序，
  // 组序即出现序。用 `new Date(y, m, d-1)` 做日历减法，避开 DST 的 24h 误差。
  const recentGroups = useMemo(() => {
    const now = new Date();
    const startToday = new Date(now.getFullYear(), now.getMonth(), now.getDate()).getTime();
    const startYest = new Date(
      now.getFullYear(),
      now.getMonth(),
      now.getDate() - 1,
    ).getTime();
    const out: { label: string; items: CatalogRecentDTO[] }[] = [];
    const idx = new Map<string, number>();
    for (const r of recent) {
      const t = new Date(r.played_at).getTime();
      let label: string;
      if (Number.isNaN(t)) label = "时间未知";
      else if (t >= startToday) label = "今天";
      else if (t >= startYest) label = "昨天";
      else {
        const d = new Date(t);
        label = `${d.getMonth() + 1}月${d.getDate()}日`;
      }
      let gi = idx.get(label);
      if (gi === undefined) {
        gi = out.length;
        idx.set(label, gi);
        out.push({ label, items: [] });
      }
      out[gi].items.push(r);
    }
    return out;
  }, [recent]);

  // 正在编辑评分/备注的角色（同一时刻只开一个；chips 下方渲染编辑器）。
  const [editingChar, setEditingChar] = useState<{
    gameID: string;
    name: string;
  } | null>(null);

  // 每游戏说话人是否展开全部（默认仍 top 8；聚合是全量的，长列表按需展开）。
  const [speakerExpanded, setSpeakerExpanded] = useState<Set<string>>(new Set());
  const toggleSpeakerGroup = (gameID: string) =>
    setSpeakerExpanded((prev) => {
      const next = new Set(prev);
      if (next.has(gameID)) next.delete(gameID);
      else next.add(gameID);
      return next;
    });

  if (!loaded) {
    return (
      <div className="flex flex-1 items-center justify-center text-sm text-muted-foreground">
        <Loader2 className="mr-2 h-4 w-4 animate-spin" />
        正在汇总全部游戏…
      </div>
    );
  }

  return (
    <div className="flex flex-1 flex-col overflow-hidden">
      {/* 同步/索引告警：部分成功也明示（FR-12 不静默） */}
      {(warnings.length > 0 || indexing) && (
        <div className="shrink-0 space-y-1 border-b border-amber-900/60 bg-amber-950/40 px-5 py-2 text-xs text-amber-300">
          {indexing && (
            <div className="flex items-center gap-2">
              <Loader2 className="h-3.5 w-3.5 animate-spin" />
              正在抽取台词：{indexing.title}（{indexing.done + 1}/{indexing.total}）…
            </div>
          )}
          {warnings.map((w, i) => (
            <div key={i} className="break-all">
              {w}
            </div>
          ))}
        </div>
      )}

      <main className="flex-1 space-y-6 overflow-y-auto p-5">
        {/* 收藏（event 级；星标同款数据） */}
        {favItems.length > 0 && (
          <section className="space-y-2">
            <h2 className="flex items-center gap-1.5 text-xs font-semibold uppercase tracking-[0.16em] text-muted-foreground">
              <Star className="h-3.5 w-3.5 fill-current" />
              收藏 {favItems.length}
            </h2>
            <ul className="divide-y divide-border rounded-md border border-border">
              {favItems.map((f) => (
                <FavRow
                  key={`${f.game_id}:${f.item_id}`}
                  fav={f}
                  stale={kindOf(f.game_id, f.item_id) === null}
                  busy={busy}
                  onOpen={() => {
                    const kind = kindOf(f.game_id, f.item_id);
                    if (kind) onOpen(f.game_id, kind, f.item_id, f.title);
                  }}
                  onUnfav={() => onToggleFav(f.game_id, kindOf(f.game_id, f.item_id) ?? "cg", f.item_id)}
                  onSaveMeta={(rating, note) => onSaveFavMeta(f.game_id, f.item_id, rating, note)}
                />
              ))}
            </ul>
          </section>
        )}

        {/* 收藏的角色（character 级：点名字搜该角色台词；铅笔编辑评分/备注） */}
        {favCharItems.length > 0 && (
          <section className="space-y-2">
            <h2 className="flex items-center gap-1.5 text-xs font-semibold uppercase tracking-[0.16em] text-muted-foreground">
              <Star className="h-3.5 w-3.5 fill-current" />
              收藏的角色 {favCharItems.length}
            </h2>
            <div className="flex flex-wrap gap-2">
              {favCharItems.map((f) => {
                const meta = charMeta.get(`${f.game_id}:${f.item_id}`);
                return (
                  <span
                    key={`${f.game_id}:${f.item_id}`}
                    className="flex items-center gap-1.5 rounded-md border border-border bg-card py-1.5 pl-3 pr-1.5 text-xs"
                    title={`${f.game_title} · ${f.item_id}${meta?.note ? `\n备注：${meta.note}` : ""}`}
                  >
                    <span
                      title={`${f.game_title} · ${f.item_id}`}
                      className="max-w-[180px] truncate"
                    >
                      {f.title}
                    </span>
                    {(meta?.rating ?? 0) > 0 && (
                      <span className="text-[11px] leading-none text-amber-400" title={`评分 ${meta?.rating}/5`}>
                        ★{meta?.rating}
                      </span>
                    )}
                    {meta?.note && (
                      <span className="max-w-[120px] truncate text-[10px] text-muted-foreground" title={meta.note}>
                        {meta.note}
                      </span>
                    )}
                    <button
                      disabled={busy}
                      title="评分 / 备注"
                      onClick={() =>
                        setEditingChar((cur) =>
                          cur?.gameID === f.game_id && cur?.name === f.item_id
                            ? null
                            : { gameID: f.game_id, name: f.item_id },
                        )
                      }
                      className={cn(
                        "flex h-6 w-6 items-center justify-center rounded transition-colors",
                        editingChar?.gameID === f.game_id && editingChar?.name === f.item_id
                          ? "bg-secondary text-foreground"
                          : "text-muted-foreground/60 hover:text-foreground",
                      )}
                    >
                      <Pencil className="h-3.5 w-3.5" />
                    </button>
                    <button
                      disabled={busy}
                      title="取消收藏角色"
                      onClick={() => onToggleCharFav(f.game_id, f.item_id)}
                      className="flex h-6 w-6 items-center justify-center rounded text-amber-400 hover:text-foreground"
                    >
                      <Star className="h-3.5 w-3.5 fill-current" />
                    </button>
                  </span>
                );
              })}
            </div>
            {/* 角色评分/备注编辑器（FavMetaEditor 与事件收藏共用；一次一个） */}
            {editingChar && (
              <div className="rounded-md border border-border bg-card/40 p-3">
                <div className="mb-2 text-xs text-muted-foreground">
                  编辑角色收藏：
                  <span className="text-foreground">{editingChar.name}</span>
                  {charMeta.get(`${editingChar.gameID}:${editingChar.name}`) && (
                    <span>
                      {" · "}
                      {charMeta.get(`${editingChar.gameID}:${editingChar.name}`)!.game_title}
                    </span>
                  )}
                </div>
                <FavMetaEditor
                  rating={charMeta.get(`${editingChar.gameID}:${editingChar.name}`)?.rating ?? 0}
                  note={charMeta.get(`${editingChar.gameID}:${editingChar.name}`)?.note ?? ""}
                  placeholder="备注（可选，如「最喜欢的角色」）"
                  busy={busy}
                  onSave={async (r, n) => {
                    await onSaveCharMeta(editingChar.gameID, editingChar.name, r, n);
                    setEditingChar(null); // 失败 reject → 保持打开（内容不丢）
                  }}
                  onCancel={() => setEditingChar(null)}
                />
              </div>
            )}
          </section>
        )}

        {/* 最近播放（按天分组；72h 冷归档清理） */}
        {recent.length > 0 && (
          <section className="space-y-2">
            <div className="flex items-center justify-between">
              <h2 className="flex items-center gap-1.5 text-xs font-semibold uppercase tracking-[0.16em] text-muted-foreground">
                <Clock className="h-3.5 w-3.5" />
                最近播放
              </h2>
              <Button
                variant="ghost"
                size="sm"
                className="h-6 text-xs text-muted-foreground"
                disabled={busy || pruningRecent}
                title="清理 3 天前（72h）的最近播放记录；不影响收藏与索引"
                onClick={() => void onPruneRecent()}
              >
                {pruningRecent ? "清理中…" : "清理 3 天前"}
              </Button>
              <Button
                variant="ghost"
                size="sm"
                className={cn(
                  "h-6 text-xs",
                  autoPrune ? "text-amber-400" : "text-muted-foreground",
                )}
                disabled={busy}
                title={
                  autoPrune
                    ? "启动时自动清理 3 天前的最近播放（点击关闭）"
                    : "开启：每次启动自动清理 3 天前的最近播放"
                }
                onClick={onToggleAutoPrune}
              >
                自动清理 {autoPrune ? "开" : "关"}
              </Button>
            </div>
            {recentGroups.map((g) => (
              <div key={g.label} className="space-y-1.5">
                <div className="text-[10px] text-muted-foreground">
                  {g.label} · {g.items.length} 条
                </div>
                <div className="flex gap-2 overflow-x-auto pb-1">
                  {g.items.map((r) => (
                    <button
                      key={`${r.game_id}:${r.kind}:${r.event_id}`}
                      disabled={busy}
                      onClick={() => onOpen(r.game_id, r.kind, r.event_id, r.title)}
                      className="shrink-0 rounded-md border border-border bg-card px-3 py-1.5 text-left text-xs transition-colors hover:border-primary/60 disabled:opacity-50"
                      title={`${r.game_title}\n${r.title}\n播放 ${r.play_count} 次`}
                    >
                      <span className="block max-w-[180px] truncate font-medium">{r.title}</span>
                      <span className="block max-w-[180px] truncate text-muted-foreground">
                        {r.game_title} · ×{r.play_count}
                      </span>
                    </button>
                  ))}
                </div>
              </div>
            ))}
          </section>
        )}

        {/* 角色（说话人聚合，每游戏 top 8；点名字搜台词，星标收藏） */}
        {speakerGroups.length > 0 && (
          <section className="space-y-3">
            <div className="flex items-center justify-between">
              <h2 className="flex items-center gap-1.5 text-xs font-semibold uppercase tracking-[0.16em] text-muted-foreground">
                <Users className="h-3.5 w-3.5" />
                角色
              </h2>
              {!textIndexed && (
                <Button size="sm" variant="outline" onClick={onIndexAll} disabled={busy}>
                  抽取台词以显示角色
                </Button>
              )}
            </div>
            {speakerGroups.map((sg) => {
              const expanded = speakerExpanded.has(sg.game_id);
              const shown = expanded ? sg.items : sg.items.slice(0, 8);
              return (
              <div key={sg.game_id} className="space-y-1.5">
                <div className="text-[10px] text-muted-foreground">
                  {sg.game_title} · {sg.items.length} 位
                </div>
                <div className="flex flex-wrap gap-2">
                  {shown.map((s) => {
                    const fav = favChars.has(`character|${s.game_id}|${s.name}`);
                    const rated = charMeta.get(`${s.game_id}:${s.name}`)?.rating ?? 0;
                    return (
                      <span
                        key={s.name}
                        className="group flex items-center gap-1 rounded-md border border-border bg-card py-1 pl-3 pr-1 text-xs"
                      >
                        <span
                          title={`「${s.name}」共 ${s.lines} 条台词 / ${s.events} 个事件`}
                          className="max-w-[160px] truncate"
                        >
                          {s.name}
                        </span>
                        <span className="text-[10px] text-muted-foreground">×{s.lines}</span>
                        {rated > 0 && (
                          <span
                            className="text-[11px] leading-none text-amber-400"
                            title={`我的评分 ${rated}/5`}
                          >
                            ★{rated}
                          </span>
                        )}
                        <button
                          disabled={busy}
                          title={fav ? "取消收藏角色" : "收藏角色"}
                          onClick={() => onToggleCharFav(s.game_id, s.name)}
                          className={cn(
                            "flex h-6 w-6 items-center justify-center rounded transition-opacity",
                            fav
                              ? "text-amber-400 opacity-100"
                              : "text-muted-foreground/60 opacity-0 hover:text-amber-400 focus-visible:opacity-100 group-hover:opacity-100",
                            busy && "cursor-not-allowed opacity-50",
                          )}
                        >
                          <Star className={cn("h-3.5 w-3.5", fav && "fill-current")} />
                        </button>
                      </span>
                    );
                  })}
                </div>
                {sg.items.length > 8 && (
                  <button
                    onClick={() => toggleSpeakerGroup(sg.game_id)}
                    className="text-[10px] text-muted-foreground underline-offset-2 hover:text-primary hover:underline"
                  >
                    {expanded ? "收起（显示全部）" : `展开全部 ${sg.items.length} 位`}
                  </button>
                )}
              </div>
              );
            })}
          </section>
        )}

        {groups.length === 0 ? (
          <div className="py-24 text-center text-sm text-muted-foreground">
            {filter.trim()
              ? "没有匹配的游戏或事件。换个关键词，或清空搜索框看全部。"
              : "还没有任何事件。从左侧游戏栏底部「添加游戏」登记解包目录。"}
          </div>
        ) : (
          groups.map((g) => {
            const cgN = g.items.filter((e) => e.kind === "cg").length;
            const scN = g.items.filter((e) => e.kind === "scene").length;
            const chN = g.items.filter((e) => e.kind === "chapter").length;
            const lines = textCounts[g.game.game_id] ?? 0;
            return (
              <section key={g.game.game_id} className="space-y-3">
                <div className="flex items-baseline gap-2">
                  <h2 className="text-sm font-semibold">{g.game.game_title}</h2>
                  <span
                    className="truncate text-[10px] text-muted-foreground"
                    title={g.game.game_root}
                  >
                    {shortRoot(g.game.game_root)}
                  </span>
                  <span className="flex-1" />
                  <span className="shrink-0 text-[10px] text-muted-foreground">
                    {cgN > 0 && `CG ${cgN} · `}
                    {scN > 0 && `场景 ${scN} · `}
                    {chN > 0 && `段落 ${chN} · `}
                    台词 {lines}
                  </span>
                </div>
                <div
                  className="grid auto-rows-max gap-4"
                  style={{ gridTemplateColumns: "repeat(auto-fill, minmax(220px, 1fr))" }}
                >
                  {g.items.map((e) => {
                    const fav = favs.has(`event|${e.game_id}|${e.id}`);
                    return (
                      <div key={`${e.game_id}:${e.kind}:${e.id}`} className="relative group">
                        <EventCard
                          card={cardFor(e)}
                          gameID={e.game_id}
                          lookup={onCover}
                          onClick={() => onOpen(e.game_id, e.kind, e.id, e.title)}
                        />
                        {/* 星标是卡的兄弟节点（button 嵌 button 不合法）；默认隐显，
                            hover/focus 可见，已收藏常驻（FR-12：状态不静默） */}
                        <button
                          onClick={(ev) => {
                            ev.stopPropagation();
                            onToggleFav(e.game_id, e.kind, e.id);
                          }}
                          disabled={busy}
                          title={fav ? "取消收藏" : "收藏本事件"}
                          className={cn(
                            "absolute right-1.5 top-1.5 z-10 flex h-7 w-7 items-center justify-center rounded-md border transition-opacity",
                            fav
                              ? "border-amber-700/60 bg-amber-950/80 text-amber-400 opacity-100"
                              : "border-border/60 bg-background/70 text-muted-foreground/70 opacity-0 hover:text-amber-400 focus-visible:opacity-100 group-hover:opacity-100",
                            busy && "cursor-not-allowed opacity-50",
                          )}
                        >
                          <Star className={cn("h-3.5 w-3.5", fav && "fill-current")} />
                        </button>
                      </div>
                    );
                  })}
                </div>
              </section>
            );
          })
        )}
      </main>
    </div>
  );
}

// ---------- 收藏行（评分 / 备注行内编辑） ----------

/** 一行收藏：打开 / 评分（1~5，再点当前星清零）/ 备注 / 取消收藏。 */
function FavRow({
  fav,
  stale,
  busy,
  onOpen,
  onUnfav,
  onSaveMeta,
}: {
  fav: FavoriteDTO;
  stale: boolean;
  busy: boolean;
  onOpen: () => void;
  onUnfav: () => void;
  onSaveMeta: (rating: number, note: string) => Promise<void>;
}) {
  const [editing, setEditing] = useState(false);

  return (
    <li className="px-4 py-2.5">
      <div className="flex items-center gap-3">
        <Star className="h-3.5 w-3.5 shrink-0 fill-current text-amber-400" />
        <button
          disabled={busy || stale}
          onClick={onOpen}
          title={stale ? "事件已不在当前库中（可取消收藏）" : `${fav.game_title}\n${fav.title}`}
          className={cn(
            "min-w-0 flex-1 text-left text-sm",
            stale ? "cursor-not-allowed text-muted-foreground line-through" : "hover:text-primary",
          )}
        >
          <span className="block truncate">{fav.title}</span>
          <span className="block truncate text-[10px] text-muted-foreground">{fav.game_title}</span>
        </button>
        {fav.rating > 0 && (
          <span className="shrink-0 text-[11px] leading-none" title={`评分 ${fav.rating}/5`}>
            <span className="text-amber-400">{"★".repeat(fav.rating)}</span>
            <span className="text-muted-foreground/40">{"★".repeat(5 - fav.rating)}</span>
          </span>
        )}
        <Button
          variant="ghost"
          size="sm"
          className="shrink-0 text-xs text-muted-foreground"
          disabled={busy}
          title="评分 / 备注"
          onClick={() => setEditing(true)}
        >
          编辑
        </Button>
        <Button
          variant="ghost"
          size="sm"
          className="shrink-0 text-xs text-muted-foreground"
          disabled={busy}
          title="取消收藏"
          onClick={onUnfav}
        >
          取消收藏
        </Button>
      </div>
      {fav.note && !editing && (
        <p className="mt-1 truncate pl-6 text-[11px] text-muted-foreground" title={fav.note}>
          备注：{fav.note}
        </p>
      )}
      {editing && (
        <FavMetaEditor
          rating={fav.rating}
          note={fav.note}
          placeholder="备注（可选，如「海边结局」）"
          busy={busy}
          onSave={async (r, n) => {
            await onSaveMeta(r, n);
            setEditing(false); // 失败会 reject → 保持编辑态继续改（错误由上层 warnings 展示）
          }}
          onCancel={() => setEditing(false)}
        />
      )}
    </li>
  );
}

// ---------- 收藏元数据编辑器（事件/角色通用） ----------

/**
 * 评分 + 备注行内编辑器：5 星点选（再点当前星清零 = 未评分）、备注输入。
 * onSave reject 时**不自动关闭**（由调用方决定：失败应保持编辑态，
 * 用户刚填的内容不丢；错误由上层 warnings 横幅展示）。
 */
function FavMetaEditor({
  rating: initRating,
  note: initNote,
  placeholder,
  busy,
  onSave,
  onCancel,
}: {
  rating: number;
  note: string;
  placeholder: string;
  busy: boolean;
  onSave: (rating: number, note: string) => Promise<void>;
  onCancel: () => void;
}) {
  const [rating, setRating] = useState(initRating);
  const [note, setNote] = useState(initNote);
  const [saving, setSaving] = useState(false);

  const save = async () => {
    setSaving(true);
    try {
      await onSave(rating, note);
    } catch {
      /* 上层已记 warning；保持编辑态 */
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="mt-2 space-y-2 rounded-md border border-border bg-background/40 p-3">
      <div className="flex items-center gap-1">
        <span className="mr-2 text-[11px] text-muted-foreground">评分</span>
        {[1, 2, 3, 4, 5].map((n) => (
          <button
            key={n}
            disabled={busy || saving}
            onClick={() => setRating((r) => (r === n ? 0 : n))} // 再点当前星 = 取消评分
            title={n === rating ? "再点一次取消评分" : `${n} 星`}
            className="p-0.5"
          >
            <Star
              className={cn(
                "h-4 w-4",
                n <= rating ? "fill-current text-amber-400" : "text-muted-foreground/40",
              )}
            />
          </button>
        ))}
        <span className="ml-2 text-[11px] text-muted-foreground">
          {rating > 0 ? `${rating}/5` : "未评分"}
        </span>
      </div>
      <input
        value={note}
        onChange={(e) => setNote(e.target.value)}
        placeholder={placeholder}
        className="h-8 w-full rounded-md border border-input bg-input/40 px-2 text-xs outline-none placeholder:text-muted-foreground focus:ring-1 focus:ring-ring"
      />
      <div className="flex justify-end gap-2">
        <Button variant="ghost" size="sm" disabled={saving} onClick={onCancel}>
          取消
        </Button>
        <Button size="sm" disabled={busy || saving} onClick={() => void save()}>
          {saving ? "保存中…" : "保存"}
        </Button>
      </div>
    </div>
  );
}

// ---------- 卡片 ----------
function EventCard({
  card,
  gameID,
  lookup,
  onClick,
}: {
  card: CardItem;
  gameID: string;
  lookup: (gameID: string, kind: string, id: string) => Promise<string | null>;
  onClick: () => void;
}) {
  const [cover, setCover] = useState<string | null>(null);
  const [miss, setMiss] = useState(false); // 封面缺失：角标可见，不静默
  useEffect(() => {
    let alive = true;
    void lookup(gameID, card.kind, card.id).then((u) => {
      if (!alive) return;
      setCover(u);
      if (!u) setMiss(true);
    });
    return () => {
      alive = false;
    };
  }, [gameID, card.kind, card.id]);

  return (
    <button
      onClick={onClick}
      className="group relative overflow-hidden rounded-lg border border-border bg-card text-left transition-shadow hover:border-primary/60 hover:shadow-md"
    >
      <div className="relative aspect-[220/124] w-full overflow-hidden bg-secondary/60">
        {cover ? (
          <img src={cover} alt="" className="h-full w-full object-cover" draggable={false} />
        ) : (
          <div className="flex h-full w-full items-center justify-center bg-gradient-to-br from-secondary to-accent/40 text-muted-foreground/40">
            <Ship className="h-8 w-8" />
          </div>
        )}
        {miss && (
          <span className="absolute left-1.5 top-1.5 rounded border border-amber-900 bg-amber-950/80 px-1 py-0.5 text-[10px] text-amber-400">
            无封面
          </span>
        )}
      </div>
      <div className="space-y-1 px-3 py-2">
        <div className="truncate text-xs text-muted-foreground">{card.id}</div>
        <div className="truncate text-sm">{card.title}</div>
        {card.note && (
          <Badge variant="secondary" className="mt-0.5">
            {card.note}
          </Badge>
        )}
      </div>
    </button>
  );
}
