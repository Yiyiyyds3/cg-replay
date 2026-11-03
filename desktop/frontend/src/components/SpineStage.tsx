// SpineStage —— 骨骼动画舞台（Spine WebGL）。
//
// Go 侧不渲染 Spine，只把 skeleton/atlas/贴图（dataURL）一次性塞进
// SessionMeta；这里按 FrameDTO.now 同步动画时间，画面直接画在 canvas 上。
// 暂停/拖动进度条时由 now 的跳变重置 TrackEntry.trackTime，避免动画自己跑飞。

import { useEffect, useRef } from "react";
import {
  AnimationState,
  AnimationStateData,
  AtlasAttachmentLoader,
  GLTexture,
  ManagedWebGLRenderingContext,
  Physics,
  ResizeMode,
  SceneRenderer,
  Skeleton,
  SkeletonJson,
  TextureAtlas,
} from "@esotericsoftware/spine-webgl";
import type { SpineDTO } from "@/lib/api";

function loadImage(src: string): Promise<HTMLImageElement> {
  return new Promise((resolve, reject) => {
    const img = new Image();
    img.onload = () => resolve(img);
    img.onerror = () => reject(new Error("贴图解码失败"));
    img.src = src;
  });
}

function pageTextureURL(spine: SpineDTO, pageName: string): string | undefined {
  const exact = spine.textures.find((t) => t.name === pageName);
  if (exact) return exact.data_url;
  const base = pageName.replace(/\\/g, "/").split("/").pop() ?? pageName;
  return spine.textures.find((t) => t.name === base || t.name.toLowerCase() === base.toLowerCase())
    ?.data_url;
}

export default function SpineStage({
  spine,
  now,
  playing,
  done,
  onError,
}: {
  spine: SpineDTO;
  now: number;
  playing: boolean;
  done: boolean;
  onError: (msg: string) => void;
}) {
  const canvasRef = useRef<HTMLCanvasElement | null>(null);
  const nowRef = useRef(now);
  const playingRef = useRef(playing);
  const doneRef = useRef(done);
  const errRef = useRef(onError);
  nowRef.current = now;
  playingRef.current = playing;
  doneRef.current = done;
  errRef.current = onError;

  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    let disposed = false;
    let raf = 0;
    let renderer: SceneRenderer | undefined;
    let atlas: TextureAtlas | undefined;
    let gl: ManagedWebGLRenderingContext | undefined;
    let ro: ResizeObserver | undefined;

    void (async () => {
      gl = new ManagedWebGLRenderingContext(canvas, {
        alpha: true,
        premultipliedAlpha: false,
        preserveDrawingBuffer: true,
      });
      renderer = new SceneRenderer(canvas, gl);

      atlas = new TextureAtlas(spine.atlas);
      for (const page of atlas.pages) {
        const url = pageTextureURL(spine, page.name);
        if (!url) throw new Error(`atlas 贴图 ${page.name} 没有随会话下发`);
        const img = await loadImage(url);
        if (disposed) return;
        page.setTexture(new GLTexture(gl, img, false));
      }

      const data = new SkeletonJson(new AtlasAttachmentLoader(atlas)).readSkeletonData(
        spine.skeleton,
      );
      const skeleton = new Skeleton(data);
      const state = new AnimationState(new AnimationStateData(data));
      const physics = Physics.update; // 有物理约束时正常推进；没有也不报错

      let animName = spine.animation;
      let loop = spine.loop;
      if (!animName) {
        animName = data.animations[0]?.name ?? "";
        loop = true; // 索引缺统计时按循环播，至少不会停在一帧
      }
      const entry = animName ? state.setAnimation(0, animName, loop) : null;

      if (entry) {
        const t = Math.max(0, nowRef.current / 1000);
        entry.trackTime = loop && entry.animation && entry.animation.duration > 0
          ? t % entry.animation.duration
          : t;
      }
      state.update(0);
      state.apply(skeleton);

      const w = data.width > 0 ? data.width : 1600;
      const h = data.height > 0 ? data.height : 1200;
      renderer.camera.position.set(data.x + w / 2, data.y + h / 2, 0);
      renderer.camera.viewportWidth = w;
      renderer.camera.viewportHeight = h;
      renderer.camera.update();

      const resize = () => renderer?.resize(ResizeMode.Fit);
      resize();
      if (typeof ResizeObserver !== "undefined") {
        ro = new ResizeObserver(resize);
        ro.observe(canvas);
      }

      let lastNow = nowRef.current;
      const tick = () => {
        if (disposed) return;
        const target = nowRef.current;
        const delta = Math.max(0, Math.min(0.2, (target - lastNow) / 1000));
        lastNow = target;

        if (entry) {
          const want = Math.max(0, target / 1000);
          const animDur = entry.animation?.duration ?? 0;
          const diff = entry.trackTime - want;
          const wrapped =
            loop && animDur > 0
              ? Math.abs(((diff % animDur) + animDur) % animDur)
              : Math.abs(diff);
          if (wrapped > 0.35) {
            // 拖动进度条/暂停后跳转：直接对齐，不补播中间几秒。
            entry.trackTime = loop && animDur > 0 ? want % animDur : want;
          } else if (playingRef.current && !doneRef.current) {
            state.update(delta);
          }
        } else {
          state.update(0);
        }
        state.apply(skeleton);
        skeleton.updateWorldTransform(physics);
        renderer?.begin();
        renderer?.drawSkeleton(skeleton);
        renderer?.end();
        raf = requestAnimationFrame(tick);
      };
      tick();
    })().catch((e: unknown) => {
      if (!disposed) {
        const msg = e instanceof Error ? e.message : String(e);
        errRef.current(`Spine 渲染失败：${msg}`);
      }
    });

    return () => {
      disposed = true;
      cancelAnimationFrame(raf);
      ro?.disconnect();
      atlas?.dispose();
      renderer?.dispose();
      gl?.dispose();
    };
  }, [spine]);

  return (
    <canvas
      ref={canvasRef}
      width={1600}
      height={1200}
      className="max-h-full max-w-full object-contain"
      draggable={false}
    />
  );
}
