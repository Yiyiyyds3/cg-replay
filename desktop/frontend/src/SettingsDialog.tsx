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
}: {
  open: boolean;
  onClose: () => void;
  extractVideo: boolean;
  onChangeExtractVideo: (v: boolean) => void;
  autoPrune: boolean;
  onToggleAutoPrune: () => void;
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

        <div className="max-h-[70vh] space-y-5 overflow-y-auto px-5 py-4">
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
                <span className="mt-0.5 block text-[11px] leading-relaxed text-muted-foreground">
                  影片很占空间（单个 100~300 MB，一部作品常常 1~2 GB）。关掉后影片事件仍会出现在事件库里，
                  但播放时缺素材；<b>已经解包的游戏需要重新解包才会补上</b>。
                </span>
              </span>
            </label>
            <p className="pl-6 text-[11px] text-muted-foreground/80">
              解包预览面板里还有一个同样的勾选框，用于那一次导入临时覆盖这个设置。
            </p>
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
                <span className="block text-sm">启动时自动清理 3 天前的最近播放</span>
                <span className="mt-0.5 block text-[11px] leading-relaxed text-muted-foreground">
                  只清理「最近播放」记录，不动收藏、评分与备注。
                </span>
              </span>
            </label>
          </section>
        </div>

        <div className="flex items-center justify-between border-t border-border px-5 py-3">
          <span className="text-[11px] text-muted-foreground">设置会自动保存</span>
          <Button size="sm" onClick={onClose}>
            完成
          </Button>
        </div>
      </div>
    </div>
  );
}
