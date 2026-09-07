import { test, expect } from "@playwright/test";

// Smoke test for "batch send-to-board" (issue #76): multi-select assets in the
// list view, send them to a board via the batch bar, and verify the board
// receives them with a deterministic non-overlapping layout. The selection and
// the 发往图板 menu are driven through the UI (the feature under test); the final
// board state is asserted via the API (authoritative) plus a screenshot.

const API = "/api/dam";

interface Asset {
  id: string;
  name: string;
}
interface BoardItem {
  id: string;
  asset_id: string;
  x: number;
  y: number;
  w: number;
  h: number;
}
interface Board {
  id: string;
  name: string;
  items?: BoardItem[];
}

test.describe("Batch send-to-board smoke", () => {
  test("multi-select → 发往图板 → existing board receives assets", async ({
    page,
    request,
  }) => {
    const res = await request.get(`${API}/assets?limit=10`);
    expect(res.ok()).toBeTruthy();
    const assets: Asset[] = await res.json();
    expect(assets.length).toBeGreaterThanOrEqual(3);

    // Pre-create a uniquely named target board so we can find it deterministically.
    const boardName = `SendTarget-${Date.now()}`;
    const createRes = await request.post(`${API}/boards`, { data: { name: boardName } });
    expect(createRes.ok()).toBeTruthy();
    const board: Board = await createRes.json();

    await page.goto("/");
    await expect(page.getByTestId("grid-view")).toBeVisible();

    // Select three assets via their card checkboxes.
    const checks = page.getByTestId("grid-view").locator('button[title="选择"]');
    await expect(checks.nth(2)).toBeVisible();
    for (let i = 0; i < 3; i++) await checks.nth(i).click();

    // The batch bar reflects the selection.
    await expect(page.getByText(/已选/)).toBeVisible();

    // Open 发往图板 and pick the pre-created board.
    await page.getByRole("button", { name: "发往图板" }).click();
    await expect(page.getByRole("button", { name: boardName })).toBeVisible();
    await page.screenshot({ path: "/tmp/hetu-e2e/01-send-menu.png" });
    await page.getByRole("button", { name: boardName }).click();

    // Success feedback names how many landed.
    await expect(page.getByText(/已添加 3 张到/)).toBeVisible();

    // Authoritative: the board now holds three assets at distinct positions.
    const afterRes = await request.get(`${API}/boards/${board.id}`);
    const after: Board = await afterRes.json();
    expect(after.items?.length).toBe(3);
    const positions = new Set(after.items!.map((it) => `${it.x},${it.y}`));
    expect(positions.size).toBe(3);
    for (const it of after.items!) {
      expect(it.w).toBe(200);
      expect(it.h).toBe(200);
    }

    // Open the board in the UI and screenshot as visual evidence.
    await page.getByText("图板", { exact: true }).click();
    await expect(page.getByText("新建图板")).toBeVisible();
    await page.locator("[class*='card']").filter({ hasText: boardName }).first().click();
    await expect(page.getByText("返回图板列表")).toBeVisible({ timeout: 5000 });
    await page.waitForTimeout(600); // let the Konva canvas paint the items
    await page.screenshot({ path: "/tmp/hetu-e2e/02-board-with-items.png" });
  });

  test("发往图板 → 新建图板 creates a board and sends the selection", async ({
    page,
    request,
  }) => {
    const before = (await (await request.get(`${API}/boards`)).json()) as Board[];

    await page.goto("/");
    await expect(page.getByTestId("grid-view")).toBeVisible();

    const checks = page.getByTestId("grid-view").locator('button[title="选择"]');
    await expect(checks.nth(1)).toBeVisible();
    for (let i = 0; i < 2; i++) await checks.nth(i).click();

    await page.getByRole("button", { name: "发往图板" }).click();
    await page.getByRole("button", { name: "新建图板…" }).click();

    await expect(page.getByText(/已添加 2 张到/)).toBeVisible();

    // A new board appeared and holds the two sent assets.
    const afterBoards = (await (await request.get(`${API}/boards`)).json()) as Board[];
    expect(afterBoards.length).toBe(before.length + 1);
    const beforeIds = new Set(before.map((b) => b.id));
    const created = afterBoards.find((b) => !beforeIds.has(b.id));
    expect(created).toBeDefined();
    const detail = (await (await request.get(`${API}/boards/${created!.id}`)).json()) as Board;
    expect(detail.items?.length).toBe(2);
  });
});
