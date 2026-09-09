import { test, expect, type APIRequestContext, type Page } from "@playwright/test";

// Smoke test for Issue #55: the Collections frontend. Collection + child create,
// membership, view, reorder, cover, remove, and cascade delete are exercised
// through the real UI where practical; assertions read the REST API (like
// boards.spec.ts / facets.spec.ts). Native HTML5 drag-and-drop can't be driven by
// Playwright's mouse, so drops are dispatched as synthetic DragEvents carrying a
// text/plain asset id — the exact payload the app's drag sources set (mirrors
// import.spec.ts's fireDrag).

const API = "/api/dam";

interface AssetLite {
  id: string;
  name: string;
}
interface CollectionLite {
  id: string;
  name: string;
  parent_id: string;
  cover: string;
}
interface ItemLite {
  asset_id: string;
  ord: number;
}

async function listCollections(request: APIRequestContext): Promise<CollectionLite[]> {
  const res = await request.get(`${API}/collections`);
  expect(res.ok()).toBeTruthy();
  return (await res.json()) as CollectionLite[];
}

async function listItems(request: APIRequestContext, id: string): Promise<ItemLite[]> {
  const res = await request.get(`${API}/collections/${id}/items`);
  expect(res.ok()).toBeTruthy();
  return (await res.json()) as ItemLite[];
}

const itemIds = async (request: APIRequestContext, id: string): Promise<string[]> =>
  (await listItems(request, id)).map((i) => i.asset_id);

// Dispatches a synthetic HTML5 drop of `assetId` (text/plain) onto the first
// element matching `selector`, driving the app's real onDragOver/onDrop handlers.
async function dropAssetOn(page: Page, selector: string, assetId: string): Promise<void> {
  await page.evaluate(
    ({ selector, assetId }) => {
      const target = document.querySelector(selector);
      if (!target) throw new Error(`drop target missing: ${selector}`);
      const dt = new DataTransfer();
      dt.setData("text/plain", assetId);
      const init: DragEventInit = { bubbles: true, cancelable: true, dataTransfer: dt };
      target.dispatchEvent(new DragEvent("dragover", init));
      target.dispatchEvent(new DragEvent("drop", init));
    },
    { selector, assetId },
  );
}

test.describe("Collections smoke", () => {
  let assets: AssetLite[];

  test.beforeAll(async ({ request }) => {
    // Prerequisite: the library needs at least 3 indexed assets (reorder needs 3).
    const res = await request.get(`${API}/assets?limit=10`);
    expect(res.ok()).toBeTruthy();
    assets = (await res.json()) as AssetLite[];
    expect(assets.length).toBeGreaterThanOrEqual(3);
  });

  test("create root+child → drag-add → view → reorder → cover → remove → cascade delete", async ({
    page,
    request,
  }) => {
    await page.goto("/");
    await expect(page.getByTestId("grid-view")).toBeVisible();

    const rootName = `合集-${Date.now()}`;
    const childName = `子合集-${Date.now()}`;

    // 1. Create a root collection via the sidebar section "+".
    await page.getByTitle("新建合集").click();
    await page.getByPlaceholder("合集名称").fill(rootName);
    await page.getByPlaceholder("合集名称").press("Enter");
    await expect(page.getByText(rootName, { exact: true })).toBeVisible();

    const rootId = (await listCollections(request)).find((c) => c.name === rootName)?.id;
    expect(rootId, "created root collection should exist via API").toBeTruthy();
    // Address the node by its stable id: a child's name can contain the root's
    // name as a substring (子合集-… ⊃ 合集-…), which breaks a hasText match.
    const rootNode = page.locator(`[data-collection-id="${rootId}"]`);

    // 2. Create a child collection under it (hover reveals the per-node "+").
    await rootNode.hover();
    await rootNode.getByTitle("新建子合集").click();
    await page.getByPlaceholder("子合集名称").fill(childName);
    await page.getByPlaceholder("子合集名称").press("Enter");
    const childNode = page.locator('[data-testid="collection-node"]', { hasText: childName });
    await expect(childNode).toBeVisible();
    const child = (await listCollections(request)).find((c) => c.name === childName);
    expect(child, "child collection should exist").toBeTruthy();
    expect(child?.parent_id).toBe(rootId);

    // 3. Drag three assets onto the root node (synthetic HTML5 drops).
    const three = assets.slice(0, 3).map((a) => a.id);
    for (const id of three) {
      await dropAssetOn(page, `[data-collection-id="${rootId}"]`, id);
    }
    await expect.poll(() => listItems(request, rootId!).then((i) => i.length)).toBe(3);

    // 4. Open the collection detail view; the three members render.
    await page.getByText(rootName, { exact: true }).click();
    await expect(page.getByTestId("collection-grid")).toBeVisible();
    await expect(page.getByTestId("collection-member")).toHaveCount(3);

    // 5. Reorder: drag the first member onto the third. reorderMembers moves the
    //    source into the target's slot → [a,b,c] becomes [b,c,a].
    const order = await itemIds(request, rootId!);
    await dropAssetOn(page, `[data-asset-id="${order[2]}"]`, order[0]);
    await expect
      .poll(() => itemIds(request, rootId!).then((ids) => ids.join(",")))
      .toBe([order[1], order[2], order[0]].join(","));

    // 6. Set cover: hover a member, click "设为封面"; the API reflects the override.
    const coverId = order[1];
    const coverCard = page.locator(`[data-asset-id="${coverId}"]`);
    await coverCard.hover();
    await coverCard.getByTestId("set-cover").click();
    await expect
      .poll(() => listCollections(request).then((cs) => cs.find((c) => c.id === rootId)?.cover))
      .toBe(coverId);

    // 7. Remove a member; count drops to two.
    const removeCard = page.locator(`[data-asset-id="${order[2]}"]`);
    await removeCard.hover();
    await removeCard.getByTestId("remove-member").click();
    await expect.poll(() => listItems(request, rootId!).then((i) => i.length)).toBe(2);

    // 8. Delete: go back, delete the root. Deleting a collection cascade-clears
    //    its membership rows (schema.sql), but not sub-collections — the child is
    //    orphaned and resurfaces at root level (buildCollectionTree), so delete it
    //    too to prove both the parent and (orphaned) child delete cleanly.
    await page.getByTitle("返回").click();
    await expect(page.getByTestId("grid-view")).toBeVisible();
    await rootNode.hover();
    await rootNode.getByTitle("删除").click();
    await expect
      .poll(() => listCollections(request).then((cs) => cs.some((c) => c.id === rootId)))
      .toBe(false);

    // The orphaned child is now a top-level node; remove it as well.
    const orphanNode = page.locator(`[data-collection-id="${child?.id}"]`);
    await orphanNode.hover();
    await orphanNode.getByTitle("删除").click();
    await expect
      .poll(() => listCollections(request).then((cs) => cs.some((c) => c.id === child?.id)))
      .toBe(false);
  });
});
