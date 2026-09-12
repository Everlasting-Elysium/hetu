import { test, expect, type APIRequestContext, type Page } from "@playwright/test";

// E2E for issue #62 (folder cover/color): setting a folder color shows a swatch
// in the sidebar and persists via the REST API; setting an asset inside a folder
// as its cover makes the sidebar folder row render that asset's thumbnail.
// Self-provisioning via the REST API (like favorite.spec.ts / collections.spec.ts)
// so it is robust to whatever the deploy seed holds.

const API = "/api/dam";

interface AssetLite {
  id: string;
  name: string;
}
interface FolderLite {
  id: string;
  name: string;
  cover: string;
  cover_url?: string;
  color: string;
}

async function listAssets(request: APIRequestContext): Promise<AssetLite[]> {
  const res = await request.get(`${API}/assets?limit=10`);
  expect(res.ok()).toBeTruthy();
  return (await res.json()) as AssetLite[];
}

async function listFolders(request: APIRequestContext): Promise<FolderLite[]> {
  const res = await request.get(`${API}/folders`);
  expect(res.ok()).toBeTruthy();
  return (await res.json()) as FolderLite[];
}

const findFolder = async (request: APIRequestContext, id: string): Promise<FolderLite | undefined> =>
  (await listFolders(request)).find((f) => f.id === id);

// moveToFolder assigns assets to a folder via the batch endpoint the grid uses.
async function moveToFolder(request: APIRequestContext, assetIds: string[], folderId: string) {
  const res = await request.post(`${API}/batch/move`, {
    data: { asset_ids: assetIds, folder_id: folderId },
  });
  expect(res.ok()).toBeTruthy();
}

// createFolderViaUI creates a root folder through the sidebar "+" and returns its
// server id.
async function createFolderViaUI(page: Page, request: APIRequestContext, name: string): Promise<string> {
  await page.getByTitle("新建文件夹").click();
  await page.getByPlaceholder("文件夹名称").fill(name);
  await page.getByPlaceholder("文件夹名称").press("Enter");
  await expect(page.getByText(name, { exact: true })).toBeVisible();
  const id = (await listFolders(request)).find((f) => f.name === name)?.id;
  expect(id, "created folder should exist via API").toBeTruthy();
  return id as string;
}

test.describe("Folder cover/color smoke", () => {
  test.beforeAll(async ({ request }) => {
    const assets = await listAssets(request);
    expect(assets.length).toBeGreaterThanOrEqual(1);
  });

  test("set folder color → sidebar swatch visible + API persisted", async ({ page, request }) => {
    await page.goto("/");
    await expect(page.getByTestId("grid-view")).toBeVisible();

    const name = `文件夹-color-${Date.now()}`;
    const id = await createFolderViaUI(page, request, name);
    const node = page.locator(`[data-folder-id="${id}"]`);

    // Open the color popover on the folder row and pick 红 (#e5484d).
    await node.hover();
    await node.getByTestId("folder-color-btn").click();
    await page.getByTitle("红").click();

    // The API persists the color, and the sidebar renders the color swatch.
    await expect.poll(() => findFolder(request, id).then((f) => f?.color)).toBe("#e5484d");
    await expect(node.getByTestId("folder-color")).toBeVisible();

    // Cleanup.
    await node.hover();
    await node.getByTitle("删除").click();
  });

  test("set asset as folder cover → sidebar shows its thumbnail", async ({ page, request }) => {
    const assets = await listAssets(request);
    const target = assets[0]!;

    const name = `文件夹-cover-${Date.now()}`;

    await page.goto("/");
    await expect(page.getByTestId("grid-view")).toBeVisible();
    const id = await createFolderViaUI(page, request, name);

    // Put an asset into the folder, then browse it so the grid shows its members.
    await moveToFolder(request, [target.id], id);
    const node = page.locator(`[data-folder-id="${id}"]`);
    await node.click();

    const cardEl = page.getByTestId("asset-card").filter({ hasText: target.name }).first();
    await expect(cardEl).toBeVisible();

    // Hover the card and click the "设为文件夹封面" pin.
    await cardEl.hover();
    await cardEl.getByTestId("set-folder-cover").click();

    // The API reflects the explicit cover, and the sidebar row now renders an
    // <img> thumbnail (the cover_url resolved) in place of the folder glyph.
    await expect.poll(() => findFolder(request, id).then((f) => f?.cover)).toBe(target.id);
    await expect(node.locator("img")).toBeVisible();

    // Cleanup: move the asset back to root and delete the folder.
    await moveToFolder(request, [target.id], "");
    await node.hover();
    await node.getByTitle("删除").click();
  });
});
