import { test, expect, type Page } from "@playwright/test";

// Smoke test for the board enhancements (issue #86): fullscreen toggle, text
// notes, multi-select alignment, and PNG export. The toolbar buttons are driven
// through the UI (the feature under test); item creation + alignment go through
// the API because Konva canvas drag/drop is not reliably automatable — same
// split as boards.spec.ts. Relative paths resolve against playwright.config.ts's
// baseURL (default :8080), and the `request` fixture shares it.

const API = "/api/dam";
const SHOT = process.env.SHOT_DIR ?? "e2e/screenshots";

interface BoardItem {
  id: string;
  kind?: "asset" | "note";
  asset_id: string;
  text?: string;
  x: number;
  y: number;
  w: number;
  h: number;
  rotation: number;
  z: number;
}
interface Board {
  id: string;
  name: string;
  items?: BoardItem[];
}

// Opens the named board's canvas from the sidebar. Waits gate on concrete
// elements rather than waitForLoadState("networkidle"): the library grid polls
// /assets continuously, so the page never reaches network-idle (batch-to-
// board.spec.ts uses the same element-based approach). "图板" needs exact:true so
// it does not also match "新建图板"; the board card is located by its unique name.
async function openBoard(page: Page, name: string): Promise<void> {
  await page.goto("/");
  await expect(page.getByTestId("grid-view")).toBeVisible();
  await page.getByText("图板", { exact: true }).click();
  await expect(page.getByText("新建图板")).toBeVisible();
  await page.locator("[class*='card']").filter({ hasText: name }).first().click();
  await expect(page.getByText("返回图板列表")).toBeVisible({ timeout: 5000 });
  await page.waitForTimeout(600); // let the Konva stage paint
}

test.describe("Board enhancements (#86)", () => {
  const boardName = `e2e-enh-${Date.now()}`;
  const consoleErrors: string[] = [];
  let boardId: string;

  // A fresh board seeded with one note, so the export dialog always has content
  // to measure (its resolution presets only render for a non-empty canvas).
  test.beforeAll(async ({ request }) => {
    const res = await request.post(`${API}/boards`, { data: { name: boardName } });
    expect(res.ok()).toBeTruthy();
    const board: Board = await res.json();
    boardId = board.id;
    expect(boardId).toBeTruthy();
    const seed = await request.post(`${API}/boards/${boardId}/items`, {
      data: { kind: "note", text: "Seed", x: 120, y: 120, w: 200, h: 150, rotation: 0, z: 0 },
    });
    expect(seed.ok()).toBeTruthy();
  });

  // Accumulate page console errors across the serial tests for a final gate.
  test.beforeEach(({ page }) => {
    page.on("console", (msg) => {
      if (msg.type() === "error") consoleErrors.push(msg.text());
    });
  });

  test("fullscreen button is present and clickable", async ({ page }) => {
    await openBoard(page, boardName);
    const fsBtn = page.getByTestId("fullscreen-btn");
    await expect(fsBtn).toBeVisible({ timeout: 5000 });
    await page.screenshot({ path: `${SHOT}/board-before-fullscreen.png` });
    // Headless Chromium may reject requestFullscreen (no real display); we only
    // assert the control exists and accepts a click without crashing the view.
    await fsBtn.click();
    await page.waitForTimeout(300);
    await expect(fsBtn).toBeVisible();
    await page.screenshot({ path: `${SHOT}/board-fullscreen-toggled.png` });
  });

  test("add a text note via API and verify persistence", async ({ page, request }) => {
    const addRes = await request.post(`${API}/boards/${boardId}/items`, {
      data: { kind: "note", text: "E2E Test Note", x: 100, y: 100, w: 200, h: 150, rotation: 0, z: 1 },
    });
    expect(addRes.ok()).toBeTruthy();
    const note: BoardItem = await addRes.json();
    expect(note.kind).toBe("note");
    expect(note.text).toBe("E2E Test Note");
    expect(note.asset_id).toBe("");
    expect(note.id).toBeTruthy();

    const getRes = await request.get(`${API}/boards/${boardId}`);
    expect(getRes.ok()).toBeTruthy();
    const board: Board = await getRes.json();
    const found = board.items?.find((it) => it.id === note.id);
    expect(found?.kind).toBe("note");
    expect(found?.text).toBe("E2E Test Note");

    await openBoard(page, boardName);
    await page.screenshot({ path: `${SHOT}/board-with-note.png` });
  });

  test("multi-select alignment via batch PATCH", async ({ request }) => {
    const mk = (text: string, x: number, y: number, z: number) =>
      request.post(`${API}/boards/${boardId}/items`, {
        data: { kind: "note", text, x, y, w: 120, h: 80, rotation: 0, z },
      });
    const r1 = await mk("Align A", 50, 200, 2);
    const r2 = await mk("Align B", 300, 350, 3);
    expect(r1.ok()).toBeTruthy();
    expect(r2.ok()).toBeTruthy();
    const a: BoardItem = await r1.json();
    const b: BoardItem = await r2.json();

    // Left-align: snap both x to the group minimum (50).
    const patchRes = await request.patch(`${API}/boards/${boardId}/items`, {
      data: { items: [{ ...a, x: 50 }, { ...b, x: 50 }] },
    });
    expect(patchRes.ok()).toBeTruthy();

    const verifyRes = await request.get(`${API}/boards/${boardId}`);
    const board: Board = await verifyRes.json();
    const ga = board.items?.find((it) => it.id === a.id);
    const gb = board.items?.find((it) => it.id === b.id);
    expect(ga?.x).toBe(50);
    expect(gb?.x).toBe(50);
  });

  test("invalid note/asset combos return 400", async ({ request }) => {
    const post = (data: Record<string, unknown>) =>
      request.post(`${API}/boards/${boardId}/items`, { data });
    const base = { x: 0, y: 0, w: 100, h: 100, rotation: 0, z: 0 };
    const noteWithAsset = await post({ kind: "note", asset_id: "x", text: "bad", ...base });
    const emptyNote = await post({ kind: "note", text: "", ...base });
    const assetNoId = await post({ kind: "asset", ...base });
    expect(noteWithAsset.status()).toBe(400);
    expect(emptyNote.status()).toBe(400);
    expect(assetNoId.status()).toBe(400);
  });

  test("export dialog opens with resolution presets", async ({ page }) => {
    await openBoard(page, boardName);
    const exportBtn = page.getByTestId("export-btn");
    await expect(exportBtn).toBeVisible({ timeout: 5000 });
    await exportBtn.click();
    await expect(page.getByRole("dialog", { name: "导出图板" })).toBeVisible();
    await page.screenshot({ path: `${SHOT}/board-export-dialog.png` });
    for (const label of ["1x", "2x", "4x"]) {
      await expect(page.getByRole("button", { name: label })).toBeVisible();
    }
    await expect(page.getByRole("button", { name: "导出 PNG" })).toBeVisible();
  });

  test("frame endpoint rejects a nonexistent asset", async ({ request }) => {
    // The #86 frame decoder serves only existing video assets; a nonexistent id
    // is a 404, a non-video asset a 400 — either proves the guard is wired.
    const res = await request.get(`${API}/assets/nonexistent-id/frame?ms=1000`);
    expect([400, 404]).toContain(res.status());
  });

  test("no unexpected console errors", () => {
    const unexpected = consoleErrors.filter(
      (m) => !/favicon|\/thumb|404|fullscreen|gesture/i.test(m),
    );
    expect(unexpected).toEqual([]);
  });
});
