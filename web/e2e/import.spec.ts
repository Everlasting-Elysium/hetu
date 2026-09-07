import { test, expect, type APIRequestContext, type Page } from "@playwright/test";

// Smoke tests for Issue #89: paste (Ctrl/⌘+V) + external file drag-drop import on
// the asset page. Both entry points funnel through useImport -> importFiles ->
// POST /api/dam/import (copy + index + thumbnail + dedupe server-side). The tests
// drive the real UI handlers with synthetic drag/paste events carrying a
// canvas-made PNG, then verify the asset landed via the REST API — kind/ext prove
// the backend inferred the type from the (possibly synthesized) filename.

const API = "/api/dam";

interface AssetLite {
  name: string;
  kind: string;
  ext: string;
  folder_id: string;
}

// Lists assets, retrying a transient non-2xx. A brief SQLite BUSY can surface
// while an import's follow-up job writes race this read (pre-existing backend
// behavior — the DB opens without busy_timeout/WAL — unrelated to #89), so the
// smoke stays about the import feature, not DB concurrency. A real client retries
// the same way.
async function getAssets(request: APIRequestContext): Promise<AssetLite[]> {
  let lastStatus = 0;
  for (let attempt = 0; attempt < 6; attempt++) {
    const res = await request.get(`${API}/assets?limit=500`);
    if (res.ok()) return (await res.json()) as AssetLite[];
    lastStatus = res.status();
    await new Promise((r) => setTimeout(r, 300));
  }
  throw new Error(`GET /assets not ok after retries (last status ${lastStatus})`);
}

async function assetCount(request: APIRequestContext): Promise<number> {
  return (await getAssets(request)).length;
}

// Finds an imported asset by name prefix. Deterministic (unlike "newest by
// indexed_at", which ties when several imports land in the same second) and it
// doubles as proof the filename reached the backend intact — e.g. a "pasted-"
// match can only exist if useImport synthesized the name for a nameless blob.
async function findAssetByName(
  request: APIRequestContext,
  prefix: string,
): Promise<AssetLite | undefined> {
  return (await getAssets(request)).find((a) => a.name.startsWith(prefix));
}

// Dispatches a synthetic external-file drag of one or more files onto the grid
// container. phase "enter" fires dragenter+dragover (raises the drop overlay);
// "drop" fires the drop that triggers the import. PNGs are painted in-page
// because File objects can't cross the evaluate boundary.
async function fireDrag(page: Page, phase: "enter" | "drop", names: string[]): Promise<void> {
  await page.evaluate(
    async ({ phase, names }) => {
      const dt = new DataTransfer();
      for (const name of names) {
        const canvas = document.createElement("canvas");
        canvas.width = 8;
        canvas.height = 8;
        const ctx = canvas.getContext("2d");
        if (!ctx) throw new Error("no 2d context");
        ctx.fillStyle = "#4f8ff7";
        ctx.fillRect(0, 0, 8, 8);
        const blob: Blob = await new Promise((resolve, reject) =>
          canvas.toBlob((b) => (b ? resolve(b) : reject(new Error("toBlob null"))), "image/png"),
        );
        dt.items.add(new File([blob], name, { type: "image/png" }));
      }
      const grid = document.querySelector('[data-testid="grid-view"]');
      if (!grid) throw new Error("grid missing");
      const init: DragEventInit = { bubbles: true, cancelable: true, dataTransfer: dt };
      if (phase === "enter") {
        grid.dispatchEvent(new DragEvent("dragenter", init));
        grid.dispatchEvent(new DragEvent("dragover", init));
      } else {
        grid.dispatchEvent(new DragEvent("drop", init));
      }
    },
    { phase, names },
  );
}

// Dispatches a synthetic paste carrying one PNG file with the given name (""
// simulates a nameless clipboard screenshot). clipboardData is force-defined
// because a constructed ClipboardEvent drops it in Chromium.
async function firePaste(page: Page, name: string): Promise<void> {
  await page.evaluate(async (name) => {
    const canvas = document.createElement("canvas");
    canvas.width = 8;
    canvas.height = 8;
    const ctx = canvas.getContext("2d");
    if (!ctx) throw new Error("no 2d context");
    ctx.fillStyle = "#46a758";
    ctx.fillRect(0, 0, 8, 8);
    const blob: Blob = await new Promise((resolve, reject) =>
      canvas.toBlob((b) => (b ? resolve(b) : reject(new Error("toBlob null"))), "image/png"),
    );
    const dt = new DataTransfer();
    dt.items.add(new File([blob], name, { type: "image/png" }));
    const evt = new Event("paste", { bubbles: true, cancelable: true });
    Object.defineProperty(evt, "clipboardData", { value: dt });
    window.dispatchEvent(evt);
  }, name);
}

test("external file drag-drop imports into the library + shows a drop highlight", async ({
  page,
  request,
}) => {
  await page.goto("/");
  await expect(page.getByTestId("grid-view")).toBeVisible();
  const before = await assetCount(request);

  await fireDrag(page, "enter", ["e2e-drag.png"]);
  await expect(page.getByTestId("drop-overlay")).toBeVisible();
  await page.screenshot({ path: "e2e/screenshots/import-drag-overlay.png" });

  await fireDrag(page, "drop", ["e2e-drag.png"]);
  await expect.poll(() => assetCount(request), { timeout: 15_000 }).toBe(before + 1);
  const a = await findAssetByName(request, "e2e-drag");
  expect(a, "the dropped e2e-drag.png should be imported").toBeTruthy();
  expect(a?.kind).toBe("image");
  expect(a?.ext).toBe("png");
  await expect(page.getByTestId("drop-overlay")).toBeHidden();
  await page.screenshot({ path: "e2e/screenshots/import-drag-done.png" });
});

test("Ctrl/⌘+V paste imports a nameless blob with a synthesized filename", async ({
  page,
  request,
}) => {
  await page.goto("/");
  await expect(page.getByTestId("grid-view")).toBeVisible();
  const before = await assetCount(request);

  await firePaste(page, ""); // nameless → useImport synthesizes pasted-<ts>.png
  await expect.poll(() => assetCount(request), { timeout: 15_000 }).toBe(before + 1);
  // A "pasted-" asset can only exist if useImport synthesized the name for the
  // nameless blob; kind/ext prove the backend inferred type from it.
  const a = await findAssetByName(request, "pasted-");
  expect(a, "a pasted-<ts>.png asset should exist (filename synthesis)").toBeTruthy();
  expect(a?.kind).toBe("image");
  expect(a?.ext).toBe("png");
  await page.screenshot({ path: "e2e/screenshots/import-paste-done.png" });
});

test("paste is ignored while a text input is focused", async ({ page, request }) => {
  await page.goto("/");
  await expect(page.getByTestId("grid-view")).toBeVisible();
  const before = await assetCount(request);

  // Focus the search field; a paste there belongs to the input, not the importer.
  await page.getByPlaceholder(/搜索名称/).click();
  await firePaste(page, "");
  await page.waitForTimeout(1000);
  expect(await assetCount(request)).toBe(before); // no import happened
});

test("multi-file drop imports every file and reports an aggregate count", async ({
  page,
  request,
}) => {
  await page.goto("/");
  await expect(page.getByTestId("grid-view")).toBeVisible();
  const before = await assetCount(request);

  await fireDrag(page, "drop", ["multi-a.png", "multi-b.png"]);
  // The aggregate success toast: "已导入 2 / 共 2" (no skips under keep-both).
  await expect(page.getByText("已导入 2 / 共 2")).toBeVisible();
  await page.screenshot({ path: "e2e/screenshots/import-multi-notice.png" });
  await expect.poll(() => assetCount(request), { timeout: 15_000 }).toBe(before + 2);
});

test("import lands in the active folder when one is selected", async ({ page, request }) => {
  await page.goto("/");
  await expect(page.getByTestId("grid-view")).toBeVisible();

  // Create a folder via the sidebar and select it so query.folderId is set.
  const folderName = `导入夹-${Date.now()}`;
  await page.getByTitle("新建文件夹").click();
  await page.getByPlaceholder("文件夹名称").fill(folderName);
  await page.getByTitle("创建").click();
  const folderBtn = page.getByText(folderName, { exact: true });
  await expect(folderBtn).toBeVisible();
  await folderBtn.click();

  const foldersRes = await request.get(`${API}/folders`);
  expect(foldersRes.ok()).toBeTruthy();
  const folders = (await foldersRes.json()) as { id: string; name: string }[];
  const folder = folders.find((f) => f.name === folderName);
  expect(folder, "the created folder should exist via API").toBeTruthy();

  const before = await assetCount(request);
  await fireDrag(page, "drop", ["folder-drop.png"]);
  await expect.poll(() => assetCount(request), { timeout: 15_000 }).toBe(before + 1);

  // The imported asset is moved into the selected folder (client-side move #89).
  const a = await findAssetByName(request, "folder-drop");
  expect(a, "the dropped file should be imported").toBeTruthy();
  expect(a?.folder_id).toBe(folder?.id);
});
