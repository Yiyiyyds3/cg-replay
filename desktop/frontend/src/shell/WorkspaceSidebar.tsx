// 左侧工作区侧栏：游戏库 + 当前游戏事件树，以及最近/收藏/角色/设置视图。
// 数据均由 EventGrid 持有，本组件只负责导航呈现与筛选交互。

import { useMemo, useState } from "react";
import {
  ChevronDown,
  ChevronRight,
  Clock3,
  Database,
  Gamepad2,
  Loader2,
  PackageOpen,
  PanelLeftClose,
  Plus,
  RefreshCw,
  Search,
  Settings,
  Star,
} from "lucide-react";
import type {
  CatalogRecentDTO,
  CatalogStatsDTO,
  FavoriteDTO,
  GameDTO,
  LibraryDTO,
  SpeakerDTO,
} from "@/lib/api";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import type { ActivityId } from "./WorkspaceShell";

type EventTab = "all" | "cg" | "scene" | "chapter";

const SECTION_META: Record<ActivityId, { title: string; subtitle: string }> = {
  library: { title: "资源库", subtitle: "游戏 / 事件浏览器" },
  recent: { title: "最近播放", subtitle: "按最近打开排序" },
  favorites: { title: "收藏", subtitle: "事件 / 游戏 / 角色" },
  characters: { title: "角色", subtitle: "台词聚合索引" },
  settings: { title: "设置", subtitle: "解包 / 索引 / 偏好" },
};

export function WorkspaceSidebar({
  activity,
  games,
  active,
  busy,
  busyGame,
  catalog,
  lib,
  tab,
  treeFilter,
  onTreeFilter,
  onTabChange,
  onSelectGame,
  onAddGame,
  onUnpack,
  onToggleGameFav,
  onOpenEvent,
  recent,
  favItems,
  favGames,
  favChars,
  favCharItems,
  speakers,
  extractVideo,
  autoPrune,
  onOpenSettings,
  onReload,
  onToggleFav,
  onToggleCharFav,
  onToggleSidebar,
}: {
  activity: ActivityId;
  games: GameDTO[];
  active: string;
  busy: boolean;
  busyGame: string;
  catalog: CatalogStatsDTO | null;
  lib: LibraryDTO | null;
  tab: EventTab;
  treeFilter: string;
  onTreeFilter: (value: string) => void;
  onTabChange: (tab: EventTab) => void;
  onSelectGame: (id: string) => void;
  onAddGame: () => void;
  onUnpack: () => void;
  onToggleGameFav: (id: string) => void;
  onOpenEvent: (gameID: string, kind: string, id: string, title: string) => void;
  recent: CatalogRecentDTO[];
  favItems: FavoriteDTO[];
  favGames: Set<string>;
  favChars: Set<string>;
  favCharItems: FavoriteDTO[];
  speakers: SpeakerDTO[];
  extractVideo: boolean;
  autoPrune: boolean;
  onOpenSettings: () => void;
  onReload: () => void;
  onToggleFav: (gameID: string, kind: string, id: string) => void;
  onToggleCharFav: (gameID: string, name: string) => void;
  onToggleSidebar: () => void;
}) {
  const meta = SECTION_META[activity];
  const activeGame = games.find((g) => g.id === active);
  const [gameFilter, setGameFilter] = useState("");
  const [expanded, setExpanded] = useState<Set<EventTab>>(new Set(["cg"]));

  const filteredGames = useMemo(() => {
    const q = gameFilter.trim().toLowerCase();
    if (!q) return games;
    return games.filter(
      (g) =>
        g.title.toLowerCase().includes(q) ||
        g.id.toLowerCase().includes(q) ||
        g.root.toLowerCase().includes(q),
    );
  }, [games, gameFilter]);

  const toggleGroup = (key: EventTab) => {
    setExpanded((prev) => {
      const next = new Set(prev);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      return next;
    });
    onTabChange(key);
  };

  const filterText = treeFilter.trim().toLowerCase();
  const hit = (text: string) => !filterText || text.toLowerCase().includes(filterText);

  const treeGroups = [
    {
      key: "cg" as const,
      label: "CG 鉴赏",
      items: (lib?.cgs ?? [])
        .filter((e) => hit(e.title) || hit(e.id))
        .map((e) => ({ kind: "cg", id: e.id, title: e.title, sub: e.pages?.length > 1 ? `${e.pages.length} 页` : e.id })),
    },
    {
      key: "scene" as const,
      label: "主线场景",
      items: (lib?.scenes ?? [])
        .filter((e) => hit(e.title))
        .map((e) => ({ kind: "scene", id: e.id, title: e.title, sub: e.id })),
    },
    {
      key: "chapter" as const,
      label: "演出段落",
      items: (lib?.chapters ?? [])
        .filter((e) => hit(e.title) || hit(e.scene))
        .map((e) => ({ kind: "chapter", id: e.id, title: e.title, sub: `#${e.index}` })),
    },
  ];

  const speakerGroups = useMemo(() => {
    const out: { gameID: string; title: string; items: SpeakerDTO[] }[] = [];
    const index = new Map<string, number>();
    for (const speaker of speakers) {
      let at = index.get(speaker.game_id);
      if (at === undefined) {
        at = out.length;
        index.set(speaker.game_id, at);
        out.push({ gameID: speaker.game_id, title: speaker.game_title, items: [] });
      }
      out[at].items.push(speaker);
    }
    return out;
  }, [speakers]);

  const renderGameRow = (game: GameDTO) => {
    const isActive = game.id === active;
    const pending = busyGame === game.id;
    const favorite = favGames.has(game.id);
    return (
      <div
        key={game.id}
        className={cn("sidebar-game", isActive && "is-active", busy && "is-busy")}
      >
        <button
          type="button"
          className="sidebar-game-main"
          onClick={() => onSelectGame(game.id)}
          disabled={busy}
          title={`${game.title}\n${game.root}`}
        >
          <span className="sidebar-game-icon">
            <Gamepad2 className="h-3.5 w-3.5" />
          </span>
          <span className="sidebar-game-copy">
            <span className="sidebar-game-title">{game.title}</span>
            <span className="sidebar-game-path">{shortRoot(game.root)}</span>
          </span>
          {pending && <Loader2 className="h-3 w-3 animate-spin" />}
        </button>
        <button
          type="button"
          className={cn("sidebar-star", favorite && "is-favorite")}
          onClick={() => onToggleGameFav(game.id)}
          disabled={busy}
          title={favorite ? "取消收藏游戏" : "收藏游戏"}
          aria-label={favorite ? "取消收藏游戏" : "收藏游戏"}
        >
          <Star className={cn("h-3 w-3", favorite && "fill-current")} />
        </button>
      </div>
    );
  };

  return (
    <aside className="workspace-sidebar" aria-label={meta.title}>
      <div className="sidebar-heading">
        <div className="min-w-0">
          <h2 className="sidebar-heading-title">{meta.title}</h2>
          <p className="sidebar-heading-subtitle">{meta.subtitle}</p>
        </div>
        <button
          type="button"
          className="sidebar-heading-action"
          onClick={onToggleSidebar}
          title="收起侧边栏 (Ctrl+B)"
          aria-label="收起侧边栏"
        >
          <PanelLeftClose className="h-4 w-4" />
        </button>
      </div>

      <div className="sidebar-content">
        {activity === "library" && (
          <>
            <section className="sidebar-section sidebar-games-section">
              <div className="sidebar-section-title">
                <span>游戏库</span>
                <Badge variant="secondary" className="sidebar-badge">
                  {games.length}
                </Badge>
              </div>
              {games.length > 3 && (
                <div className="sidebar-filter">
                  <Search className="sidebar-filter-icon" />
                  <input
                    value={gameFilter}
                    onChange={(e) => setGameFilter(e.target.value)}
                    placeholder="筛选游戏…"
                    className="sidebar-filter-input"
                  />
                </div>
              )}
              <div className="sidebar-game-list">
                {filteredGames.map(renderGameRow)}
                {filteredGames.length === 0 && (
                  <p className="sidebar-empty">没有匹配的游戏</p>
                )}
              </div>
            </section>

            <section className="sidebar-section sidebar-tree-section">
              <div className="sidebar-section-title">
                <span>事件浏览器</span>
                {activeGame && (
                  <span className="sidebar-current-game" title={activeGame.title}>
                    {activeGame.title}
                  </span>
                )}
              </div>
              <div className="sidebar-filter">
                <Search className="sidebar-filter-icon" />
                <input
                  value={treeFilter}
                  onChange={(e) => onTreeFilter(e.target.value)}
                  placeholder="筛选当前游戏事件…"
                  className="sidebar-filter-input"
                />
              </div>

              <div className="sidebar-tree">
                {treeGroups.map((group) => {
                  const isOpen = expanded.has(group.key);
                  const isCurrent = tab === group.key;
                  return (
                    <div key={group.key} className="sidebar-tree-group">
                      <button
                        type="button"
                        className={cn("sidebar-tree-group-row", isCurrent && "is-current")}
                        onClick={() => toggleGroup(group.key)}
                        title={`切换到${group.label}`}
                      >
                        {isOpen ? (
                          <ChevronDown className="h-3.5 w-3.5 shrink-0" />
                        ) : (
                          <ChevronRight className="h-3.5 w-3.5 shrink-0" />
                        )}
                        <span className="truncate">{group.label}</span>
                        <span className="sidebar-tree-count">{group.items.length}</span>
                      </button>
                      {isOpen && (
                        <div className="sidebar-tree-children">
                          {group.items.slice(0, 120).map((item) => (
                            <button
                              type="button"
                              key={`${item.kind}:${item.id}`}
                              className="sidebar-tree-item"
                              onClick={() => onOpenEvent(active, item.kind, item.id, item.title)}
                              title={`${item.title}\n${item.id}`}
                            >
                              <span className={`sidebar-tree-dot is-${item.kind}`} />
                              <span className="sidebar-tree-item-title">{item.title}</span>
                              <span className="sidebar-tree-item-sub">{item.sub}</span>
                            </button>
                          ))}
                          {group.items.length === 0 && (
                            <p className="sidebar-empty">没有匹配事件</p>
                          )}
                          {group.items.length > 120 && (
                            <p className="sidebar-empty">仅显示前 120 条，请用筛选缩小范围</p>
                          )}
                        </div>
                      )}
                    </div>
                  );
                })}
              </div>
            </section>
          </>
        )}

        {activity === "recent" && (
          <section className="sidebar-section">
            <div className="sidebar-section-title">
              <span>最近播放</span>
              <Badge variant="secondary" className="sidebar-badge">
                {recent.length}
              </Badge>
            </div>
            <div className="sidebar-list">
              {recent.map((item) => (
                <button
                  type="button"
                  key={`${item.game_id}:${item.kind}:${item.event_id}`}
                  className="sidebar-list-row"
                  onClick={() => onOpenEvent(item.game_id, item.kind, item.event_id, item.title)}
                  disabled={busy}
                  title={`${item.game_title}\n播放 ${item.play_count} 次`}
                >
                  <Clock3 className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
                  <span className="sidebar-list-copy">
                    <span className="sidebar-list-title">{item.title}</span>
                    <span className="sidebar-list-sub">
                      {item.game_title} · ×{item.play_count}
                    </span>
                  </span>
                </button>
              ))}
              {recent.length === 0 && <p className="sidebar-empty">暂无最近播放记录</p>}
            </div>
          </section>
        )}

        {activity === "favorites" && (
          <>
            <section className="sidebar-section">
              <div className="sidebar-section-title">
                <span>收藏的事件</span>
                <Badge variant="secondary" className="sidebar-badge">
                  {favItems.length}
                </Badge>
              </div>
              <div className="sidebar-list">
                {favItems.map((item) => (
                  <div key={`${item.game_id}:${item.item_id}`} className="sidebar-fav-row">
                    <button
                      type="button"
                      className="sidebar-list-row sidebar-fav-main"
                      onClick={() => onOpenEvent(item.game_id, "", item.item_id, item.title)}
                      title={`${item.game_title}\n${item.title}`}
                    >
                      <Star className="h-3.5 w-3.5 shrink-0 fill-current text-amber-400" />
                      <span className="sidebar-list-copy">
                        <span className="sidebar-list-title">{item.title}</span>
                        <span className="sidebar-list-sub">{item.game_title}</span>
                      </span>
                    </button>
                    <button
                      type="button"
                      className="sidebar-star is-favorite"
                      onClick={() => onToggleFav(item.game_id, "cg", item.item_id)}
                      title="取消收藏"
                      aria-label="取消收藏"
                    >
                      <Star className="h-3 w-3 fill-current" />
                    </button>
                  </div>
                ))}
                {favItems.length === 0 && <p className="sidebar-empty">还没有收藏事件</p>}
              </div>
            </section>

            <section className="sidebar-section">
              <div className="sidebar-section-title">
                <span>收藏的游戏</span>
                <Badge variant="secondary" className="sidebar-badge">
                  {games.filter((g) => favGames.has(g.id)).length}
                </Badge>
              </div>
              <div className="sidebar-list">
                {games
                  .filter((g) => favGames.has(g.id))
                  .map((g) => (
                    <button
                      type="button"
                      key={g.id}
                      className="sidebar-list-row"
                      onClick={() => onSelectGame(g.id)}
                      title={g.root}
                    >
                      <Gamepad2 className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
                      <span className="sidebar-list-copy">
                        <span className="sidebar-list-title">{g.title}</span>
                        <span className="sidebar-list-sub">{shortRoot(g.root)}</span>
                      </span>
                    </button>
                  ))}
                {games.every((g) => !favGames.has(g.id)) && (
                  <p className="sidebar-empty">在游戏行尾点星标即可收藏</p>
                )}
              </div>
            </section>

            <section className="sidebar-section">
              <div className="sidebar-section-title">
                <span>收藏的角色</span>
                <Badge variant="secondary" className="sidebar-badge">
                  {favCharItems.length}
                </Badge>
              </div>
              <div className="sidebar-chip-list">
                {favCharItems.map((item) => (
                  <span key={`${item.game_id}:${item.item_id}`} className="sidebar-chip">
                    <span className="truncate" title={`${item.game_title} · ${item.item_id}`}>
                      {item.title}
                    </span>
                    <button
                      type="button"
                      className="sidebar-star is-favorite"
                      onClick={() => onToggleCharFav(item.game_id, item.item_id)}
                      title="取消收藏角色"
                      aria-label="取消收藏角色"
                    >
                      <Star className="h-3 w-3 fill-current" />
                    </button>
                  </span>
                ))}
                {favCharItems.length === 0 && <p className="sidebar-empty">还没有收藏角色</p>}
              </div>
            </section>
          </>
        )}

        {activity === "characters" && (
          <section className="sidebar-section">
            <div className="sidebar-section-title">
              <span>台词角色</span>
              <Badge variant="secondary" className="sidebar-badge">
                {speakers.length}
              </Badge>
            </div>
            <div className="sidebar-list">
              {speakerGroups.map((group) => (
                <div key={group.gameID} className="sidebar-character-group">
                  <div className="sidebar-character-game">{group.title}</div>
                  {group.items.slice(0, 30).map((speaker) => {
                    const favorite = favChars.has(
                      `character|${speaker.game_id}|${speaker.name}`,
                    );
                    return (
                      <div key={speaker.name} className="sidebar-character-row">
                        <span className="sidebar-character-name" title={`${speaker.lines} 条台词`}>
                          {speaker.name}
                        </span>
                        <span className="sidebar-character-count">×{speaker.lines}</span>
                        <button
                          type="button"
                          className={cn("sidebar-star", favorite && "is-favorite")}
                          onClick={() => onToggleCharFav(speaker.game_id, speaker.name)}
                          title={favorite ? "取消收藏角色" : "收藏角色"}
                          aria-label={favorite ? "取消收藏角色" : "收藏角色"}
                        >
                          <Star className={cn("h-3 w-3", favorite && "fill-current")} />
                        </button>
                      </div>
                    );
                  })}
                  {group.items.length > 30 && (
                    <p className="sidebar-empty">仅显示前 30 位角色</p>
                  )}
                </div>
              ))}
              {speakers.length === 0 && (
                <p className="sidebar-empty">
                  暂无角色索引。进入「全部」页签后会自动抽取台词。
                </p>
              )}
            </div>
          </section>
        )}

        {activity === "settings" && (
          <section className="sidebar-section sidebar-settings">
            <div className="sidebar-section-title">
              <span>工作区设置</span>
            </div>
            <div className="sidebar-settings-card">
              <div className="sidebar-settings-row">
                <span>解包提取影片</span>
                <Badge variant={extractVideo ? "default" : "secondary"}>
                  {extractVideo ? "开" : "关"}
                </Badge>
              </div>
              <div className="sidebar-settings-row">
                <span>启动自动清理</span>
                <Badge variant={autoPrune ? "default" : "secondary"}>
                  {autoPrune ? "开" : "关"}
                </Badge>
              </div>
              <p className="sidebar-settings-note">
                设置与 CLI 版共用同一份 cgreplay.conf；修改会立即生效。
              </p>
            </div>
            <Button className="w-full justify-start gap-2" onClick={onOpenSettings}>
              <Settings className="h-4 w-4" />
              打开设置弹层
            </Button>
            <Button
              variant="outline"
              className="w-full justify-start gap-2"
              onClick={onReload}
              disabled={busy}
            >
              <RefreshCw className="h-4 w-4" />
              重扫当前游戏
            </Button>
            <Button
              variant="outline"
              className="w-full justify-start gap-2"
              onClick={onAddGame}
              disabled={busy}
            >
              <Plus className="h-4 w-4" />
              添加游戏目录
            </Button>
            <Button
              variant="outline"
              className="w-full justify-start gap-2"
              onClick={onUnpack}
              disabled={busy}
            >
              <PackageOpen className="h-4 w-4" />
              导入并自动解包
            </Button>
            <div className="sidebar-shortcuts">
              <div className="sidebar-shortcuts-title">快捷键</div>
              <div>
                <kbd>Ctrl</kbd> + <kbd>B</kbd> 收起侧栏
              </div>
              <div>
                <kbd>Esc</kbd> 关闭当前回放标签
              </div>
            </div>
          </section>
        )}
      </div>

      <div className="sidebar-footer">
        <div className="sidebar-index" title={catalogTitle(catalog)}>
          <Database
            className={cn(
              "h-3.5 w-3.5 shrink-0",
              catalog?.error ? "text-amber-400" : "text-primary",
            )}
          />
          {catalog?.error ? (
            <span className="truncate text-amber-400">索引失败</span>
          ) : catalog ? (
            <span className="truncate">
              索引 {catalog.events} 事件 · {catalog.games} 库
            </span>
          ) : (
            <span className="truncate">索引检查中…</span>
          )}
        </div>
        <div className="sidebar-footer-actions">
          <button
            type="button"
            className="sidebar-footer-button"
            onClick={onAddGame}
            disabled={busy}
            title="添加游戏目录"
          >
            <Plus className="h-3.5 w-3.5" />
            添加
          </button>
          <button
            type="button"
            className="sidebar-footer-button"
            onClick={onUnpack}
            disabled={busy}
            title="选择目录并自动解包"
          >
            <PackageOpen className="h-3.5 w-3.5" />
            导入
          </button>
        </div>
      </div>
    </aside>
  );
}

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

function catalogTitle(catalog: CatalogStatsDTO | null): string {
  if (!catalog) return "中心库索引检查中…";
  if (catalog.error) return `中心库索引失败：${catalog.error}`;
  return `本地 SQLite 索引\n${catalog.games} 个游戏 / ${catalog.events} 个事件 / ${catalog.text_lines} 条台词\n只存可检索元数据，回放仍读原始素材。`;
}
