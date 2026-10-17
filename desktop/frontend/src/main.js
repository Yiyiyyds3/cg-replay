// 前端骨架：调用 Go 绑定的最小示例。
// 完整界面设计见 docs/06_客户端功能与交互设计.md。
//
// Wails v3 会把 desktop 包导出的方法（如 App.ScanDirectory）
// 绑定到 window.go 对象上。

const app = document.querySelector("#app");

app.innerHTML = `
  <div style="font-family: system-ui; padding: 24px; max-width: 720px;">
    <h1>统一 CG 演出回放库</h1>
    <p>骨架版本。核心 CLI 已可用：<code>ucgr scan / probe / refs / prune / engines</code>。</p>
    <input id="dir" placeholder="解包目录路径" style="width: 70%; padding: 6px;" />
    <button id="go">S0 盘点</button>
    <pre id="out" style="white-space: pre-wrap; background: #f5f5f5; padding: 12px;"></pre>
  </div>
`;

document.querySelector("#go").addEventListener("click", async () => {
  const dir = document.querySelector("#dir").value;
  const out = document.querySelector("#out");
  out.textContent = "盘点中…";
  try {
    // window.go 由 Wails 运行时注入；骨架阶段允许为空
    const text = window.go?.main?.App?.ScanDirectory
      ? await window.go.main.App.ScanDirectory(dir)
      : "（Wails 绑定未就绪：请先 go mod tidy 并 wails build）";
    out.textContent = text;
  } catch (e) {
    out.textContent = "错误: " + e;
  }
});
