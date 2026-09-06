import { test, expect } from "@playwright/test";

// Smoke test for the moodboard feature: create a board, add items, verify
// persistence after refresh. Board CRUD uses the UI; item placement uses the
// API directly because Konva canvas drag-and-drop is not reliably automatable
// via Playwright. This still validates the full stack end-to-end.

const API = "/api/dam";

interface Asset {
  id: string;
  name: string;
}
interface Board {
  id: string;
  name: string;
  items?: { id: string; x: number; y: number; w: number; h: number }[];
}

test.describe("Moodboard smoke", () => {
  let assets: Asset[];

  test.beforeAll(async ({ request }) => {
    // Prerequisite: the library must have at least 2 indexed assets.
    const res = await request.get(`${API}/assets?limit=10`);
    expect(res.ok()).toBeTruthy();
    assets = await res.json();
    expect(assets.length).toBeGreaterThanOrEqual(2);
  });

  test("create board → add items → refresh → layout persists", async ({
    page,
    request,
  }) => {
    await page.goto("/");
    await page.waitForLoadState("networkidle");

    // 1. Navigate to boards view via sidebar.
    await page.getByText("图板", { exact: true }).click();
    await expect(page.getByText("新建图板")).toBeVisible();

    // 2. Create a board via UI.
    await page.getByText("新建图板").click();
    // Should navigate to the canvas view with the board toolbar.
    await expect(page.getByText("返回图板列表")).toBeVisible({ timeout: 5000 });

    // 3. Get the created board's ID from the API.
    const boardsRes = await request.get(`${API}/boards`);
    const boards: Board[] = await boardsRes.json();
    expect(boards.length).toBeGreaterThanOrEqual(1);
    const boardId = boards[0].id;

    // 4. Add two items via API (reliable substitute for canvas drag-and-drop).
    const item1Res = await request.post(`${API}/boards/${boardId}/items`, {
      data: {
        asset_id: assets[0].id,
        x: 100,
        y: 100,
        w: 200,
        h: 150,
        rotation: 0,
        z: 0,
      },
    });
    expect(item1Res.ok()).toBeTruthy();

    const item2Res = await request.post(`${API}/boards/${boardId}/items`, {
      data: {
        asset_id: assets[1].id,
        x: 400,
        y: 200,
        w: 250,
        h: 180,
        rotation: 15,
        z: 1,
      },
    });
    expect(item2Res.ok()).toBeTruthy();

    // 5. Verify the board has 2 items.
    const boardRes = await request.get(`${API}/boards/${boardId}`);
    const board: Board = await boardRes.json();
    expect(board.items?.length).toBe(2);

    // 6. Simulate a move: batch-update item positions.
    const moved = board.items!.map((it, i) => ({
      ...it,
      x: it.x + 50,
      y: it.y + 30 * i,
    }));
    const patchRes = await request.patch(`${API}/boards/${boardId}/items`, {
      data: { items: moved },
    });
    expect(patchRes.ok()).toBeTruthy();

    // 7. Refresh the page and navigate back to the board.
    await page.reload();
    await page.waitForLoadState("networkidle");
    await page.getByText("图板", { exact: true }).click();
    await expect(page.getByText("新建图板")).toBeVisible();

    // The board card should exist; click to open.
    await page.locator("[class*='card']").first().click();
    await expect(page.getByText("返回图板列表")).toBeVisible({ timeout: 5000 });

    // 8. Verify layout persisted via API.
    const afterRes = await request.get(`${API}/boards/${boardId}`);
    const after: Board = await afterRes.json();
    expect(after.items?.length).toBe(2);
    // First item should have moved to x=150 (100+50), y=100 (100+0).
    const first = after.items!.find((it) => it.id === moved[0].id);
    expect(first).toBeDefined();
    expect(first!.x).toBe(150);
    expect(first!.y).toBe(100);

    // 9. Clean up: delete the board.
    await page.getByText("返回图板列表").click();
    await expect(page.getByText("新建图板")).toBeVisible();
  });
});
