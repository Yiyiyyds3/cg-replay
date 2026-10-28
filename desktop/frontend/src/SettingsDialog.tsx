import { useEffect } from "react";
import { Settings, X } from "lucide-react";
import { Button } from "./components/ui/button";

/**
 * 设置弹层。
 *
 * 现在只有一个「解包」分区（提取 OP/ED 影片）和一个「播放记录」分区
 * （启动自动清理）。偏好都落 localStorage，关掉面板也保持。
 *
 * 为什么需要弹层：解包时的勾选框只在「预览某个游戏」那一刻存在，用户关掉后
 * 想再加回来就没有入口了 —— 这个弹层就是那个「以后去哪打开」。
 */
export function SettingsDialog({
  open,
  onClose,
  extractVideo,
  onChangeExtractVideo,
  autoPrune,
  onToggleAutoPrune,
  onReset,
}: {
  open: boolean;
  onClose: () => void;
  extractVideo: boolean;
  onChangeExtractVideo: (v: boolean) => void;
  autoPrune: boolean;
  onToggleAutoPrune: () => void;
  onReset: () => void;
}) {
  // Esc 关闭
  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [open, onClose]);

  if (!open) return null;

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4"
      onClick={onClose}
      role="presentation"
    >
      <div
        role="dialog"
        aria-modal="true"
        aria-label="设置"
        className="w-[520px] max-w-full overflow-hidden rounded-lg border border-border bg-card shadow-xl"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center gap-2 border-b border-border px-5 py-3">
          <Settings className="h-4 w-4 text-primary" />
          <h2 className="text-sm font-semibold">设置</h2>
          <div className="flex-1" />
          <Button variant="ghost" size="icon" className="h-7 w-7" onClick={onClose} title="关闭（Esc）">
            <X className="h-4 w-4" />
          </Button>
        </div>

        <div className="max-h-[70vh] space-y-4 overflow-y-auto px-5 py-4">
          {/* 解包 */}
          <section className="space-y-2">
            <h3 className="text-[11px] font-medium uppercase tracking-wide text-muted-foreground">解包</h3>
            <label className="flex cursor-pointer items-start gap-2.5">
              <input
                type="checkbox"
                checked={extractVideo}
                onChange={(e) => onChangeExtractVideo(e.target.checked)}
                className="mt-0.5 h-4 w-4 accent-primary"
              />
              <span className="min-w-0">
                <span className="block text-sm">提取 OP/ED 影片</span>
                <span className="mt-0.5 block text-[11px] text-muted-foreground">
                  只影响新解包；已解出的影片不受影响
                </span>
              </span>
            </label>
          </section>

          {/* 播放记录 */}
          <section className="space-y-2">
            <h3 className="text-[11px] font-medium uppercase tracking-wide text-muted-foreground">播放记录</h3>
            <label className="flex cursor-pointer items-start gap-2.5">
              <input
                type="checkbox"
                checked={autoPrune}
                onChange={onToggleAutoPrune}
                className="mt-0.5 h-4 w-4 accent-primary"
              />
              <span className="min-w-0">
                <span className="block text-sm">启动时清理 3 天前的最近播放</span>
                <span className="mt-0.5 block text-[11px] text-muted-foreground">
                  不影响收藏与评分；侧栏「最近播放」区也有这个开关
                </span>
              </span>
            </label>
          </section>
        </div>

        <div className="flex items-center justify-between border-t border-border px-5 py-3">
          <span className="text-[11px] text-muted-foreground">自动保存</span>
          <div className="flex items-center gap-2">
            <Button size="sm" variant="ghost" onClick={onReset} title="恢复默认：提取影片开、启动清理关">
              恢复默认
            </Button>
            <Button size="sm" onClick={onClose}>
              完成
            </Button>
          </div>
        </div>
      </div>
    </div>
  );
}
