import { useEffect, useMemo, useRef, useState } from "react";
import { FolderOpen, Plus, RefreshCw, Search, Ship } from "lucide-react";
import {
  api,
  type CardItem,
  type GamesDTO,
  type LibraryDTO,
} from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";

type Tab = "cg" | "scene" | "chapter";

type Status =
  | { state: "loading" }
  | { state: "setup" } // 需要 conf.Root
  | { state: "ready" }
  | { state: "error"; message: string };

// M2 事件网格（卡墙）。M3：卡片点击 → App 路由切回放页。
export default function EventGrid({
  onOpen,
}: {
  onOpen: (kind: string, id: string, title: string) => void;
}) {
  const [status, setStatus] = useState<Status>({ state: "loading" });
  const [lib, setLib] = useState<LibraryDTO | null>(null);
  const [games, setGames] = useState<GamesDTO | null>(null);
  const [tab, setTab] = useState<Tab>("cg");
  const [q, setQ] = useState("");
  const [busy, setBusy] = useState(false);
  const covers = useRef(new Map<string, string>()); // game:id → dataURL 缓存

  // 启动链：游戏登记表 → 当前游戏的库（有清单读清单，没有就扫描建库）。
  const boot = async () => {
    try {
      const gs = await api.listGames();
      setGames(gs);
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

  useEffect(() => {
    void boot();
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
    } catch (e: any) {
      setStatus({ state: "error", message: e?.message ?? String(e) });
    } finally {
      setBusy(false);
    }
  };

  const switchGame = async (id: string) => {
    if (!id || id === games?.active) return;
    setBusy(true);
    try {
      const l = await api.setActiveGame(id);
      const gs = await api.listGames();
      setGames(gs);
      covers.current.clear();
      setLib(l);
      setTab("cg");
      setStatus({ state: "ready" });
    } catch (e: any) {
      setStatus({ state: "error", message: e?.message ?? String(e) });
    } finally {
      setBusy(false);
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

  const coverOf = async (kind: string, id: string): Promise<string | null> => {
    const key = `${games?.active ?? lib?.root ?? ""}:${kind}:${id}`;
    const hit = covers.current.get(key);
    if (hit) return hit;
    try {
      const url = await api.getCover(kind, id);
      covers.current.set(key, url);
      return url;
    } catch {
      // 封面缺失不阻断卡片（回放时缺素材才红横幅，FR-12 主线不变）
      return null;
    }
  };

  if (status.state === "setup") {
    return (
      <div className="flex min-h-screen items-center justify-center">
        <div className="flex w-[460px] flex-col items-center gap-4 rounded-lg border border-border bg-card px-10 py-10 text-center">
          <FolderOpen className="h-9 w-9 text-primary" />
          <h1 className="text-lg font-semibold">先选游戏解包目录</h1>
          <p className="text-sm leading-relaxed text-muted-foreground">
            选「包含 raw\data\scenario 的那一层」。选择后会自动建立事件库。
            之后可随时从顶栏「添加游戏」登记更多游戏，配置与 CLI 版
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

  return (
    <div className="flex min-h-screen flex-col">
      {/* 顶栏：品牌 + 页签 + 搜索 + 刷新 */}
      <header className="sticky top-0 z-10 flex items-center gap-4 border-b border-border bg-background/95 px-5 py-3 backdrop-blur">
        <div className="flex items-center gap-2">
          <Ship className="h-5 w-5 text-primary" />
          {games && games.games.length > 0 ? (
            <>
              <select
                value={games.active}
                onChange={(e) => void switchGame(e.target.value)}
                disabled={busy}
                title="切换游戏库"
                className="h-9 max-w-[260px] rounded-md border border-input bg-input/40 px-2 text-sm font-semibold outline-none focus:ring-1 focus:ring-ring disabled:opacity-60"
              >
                {games.games.map((g) => (
                  <option key={g.id} value={g.id}>
                    {g.title}
                  </option>
                ))}
              </select>
              {games.games.length > 1 && (
                <Badge variant="secondary">{games.games.length} 库</Badge>
              )}
            </>
          ) : (
            <span className="font-semibold">{lib?.game ?? "CG 演出回放"}</span>
          )}
        </div>
        <nav className="flex gap-1">
          {(
            [
              ["cg", `CG鉴赏 ${lib?.cgs?.length ?? 0}`],
              ["scene", `主线场景 ${lib?.scenes?.length ?? 0}`],
              ["chapter", `演出段落 ${lib?.chapters?.length ?? 0}`],
            ] as [Tab, string][]
          ).map(([k, label]) => (
            <button
              key={k}
              onClick={() => setTab(k)}
              className={
                "rounded-md px-3 py-1.5 text-sm transition-colors " +
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
        <div className="relative">
          <Search className="absolute left-2.5 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
          <input
            value={q}
            onChange={(e) => setQ(e.target.value)}
            placeholder="搜索（cg17 / 标题 / 章节…）"
            className="h-9 w-64 rounded-md border border-input bg-input/40 pl-8 pr-3 text-sm outline-none placeholder:text-muted-foreground focus:ring-1 focus:ring-ring"
          />
        </div>
        <Button variant="outline" size="icon" title="重扫" onClick={() => void reload(true)}>
          <RefreshCw className="h-4 w-4" />
        </Button>
        <Button variant="outline" size="icon" title="添加游戏" onClick={pickAndAdd} disabled={busy}>
          <Plus className="h-4 w-4" />
        </Button>
      </header>

      {/* 错误横幅区：任何异常都常驻可见（对齐 FR-12） */}
      {banner && (
        <div className="bg-destructive px-5 py-1.5 text-sm">{banner}</div>
      )}

      {/* 卡片墙 */}
      <main className="grid flex-1 content-start gap-4 overflow-y-auto p-5"
        style={{ gridTemplateColumns: "repeat(auto-fill, minmax(220px, 1fr))" }}>
        {cards.map((c) => (
          <EventCard
            key={`${lib?.root ?? ""}:${c.kind}:${c.id}`}
            card={c}
            lookup={coverOf}
            onClick={() => onOpen(c.kind, c.id, c.title)}
          />
        ))}
        {cards.length === 0 && (
          <div className="col-span-full py-24 text-center text-sm text-muted-foreground">
            没有匹配的事件
          </div>
        )}
      </main>
    </div>
  );
}

// ---------- 卡片 ----------
function EventCard({
  card,
  lookup,
  onClick,
}: {
  card: CardItem;
  lookup: (kind: string, id: string) => Promise<string | null>;
  onClick: () => void;
}) {
  const [cover, setCover] = useState<string | null>(null);
  const [miss, setMiss] = useState(false); // 封面缺失：角标可见，不静默
  useEffect(() => {
    let alive = true;
    void lookup(card.kind, card.id).then((u) => {
      if (!alive) return;
      setCover(u);
      if (!u) setMiss(true);
    });
    return () => {
      alive = false;
    };
  }, [card.kind, card.id]);

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
