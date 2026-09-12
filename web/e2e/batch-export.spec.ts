import { test, expect, type APIRequestContext, type Page } from "@playwright/test";

// E2E for issue #62 (batch export): select a few assets in the grid, click the
// batch bar's 导出 button, and assert the browser download fires with the
// assets.zip filename. Mirrors favorite/tag-merge specs — it reads the live
// library over the REST API (robust to whatever the deploy seed holds) and then
// drives the real UI. Nothing is mutated: export is a read-only packaging op.

const API = "/api/dam";

interface AssetLite {
  id: string;
  name: string;
  display_name: string;
}

async function listAssets(request: APIRequestContext): Promise<AssetLite[]> {
  const res = await request.get(`${API}/assets?limit=50`);
  expect(res.ok()).toBeTruthy();
  return (await res.json()) as AssetLite[];
}

// selectCard hovers a grid card (by its rendered label) and clicks its select
// affordance, adding it to the batch selection.
async function selectCard(page: Page, label: string) {
  const card = page.getByTestId("asset-card").filter({ hasText: label }).first();
  await card.hover();
  await card.getByTitle("选择").first().click();
}

test("batch export triggers an assets.zip download", async ({ page, request }) => {
  const assets = await listAssets(request);
  expect(assets.length, "need at least two assets to export").toBeGreaterThanOrEqual(2);
  const labelA = assets[0]!.display_name || assets[0]!.name;
  const labelB = assets[1]!.display_name || assets[1]!.name;
  expect(labelA, "two distinct asset labels are needed").not.toBe(labelB);

  await page.goto("/");
  await expect(page.getByTestId("grid-view")).toBeVisible();

  // Select two assets → the batch bar (carrying 导出) appears.
  await selectCard(page, labelA);
  await selectCard(page, labelB);
  await expect(page.getByText(/已选/)).toBeVisible();

  // Click 导出 and assert the browser download fires with the assets.zip name.
  // waitForEvent is armed before the click so the blob <a download> can't race it.
  const downloadPromise = page.waitForEvent("download");
  await page.getByTestId("batch-export").click();
  const download = await downloadPromise;
  expect(download.suggestedFilename()).toBe("assets.zip");
  await page.screenshot({ path: "e2e/screenshots/batch-export.png" });
});
