import { test, expect, type APIRequestContext, type Page } from "@playwright/test";

// E2E for issue #62 (tag merge / batch replace). Two complementary operations:
//   1. Merge: drag one tag onto another → confirm → the source tag disappears
//      globally and every asset that had it now carries the target tag.
//   2. Batch replace: with a subset selected, swap tag X for tag Y on ONLY those
//      assets; the source tag survives (unselected assets keep it).
// Self-provisions tags/asset_tags through the REST API (like collections.spec.ts
// / favorite.spec.ts) so it is robust to whatever the deploy seed holds, and
// cleans up its tags in a finally block. Native HTML5 DnD can't be driven by
// Playwright's mouse, so the tag drop is dispatched as a synthetic DragEvent
// carrying the app's own "application/x-hetu-tag" payload (mirrors the text/plain
// drops in collections.spec.ts).

const API = "/api/dam";

interface AssetLite {
  id: string;
  name: string;
  display_name: string;
}
interface TagLite {
  id: string;
  name: string;
}

async function listAssets(request: APIRequestContext): Promise<AssetLite[]> {
  const res = await request.get(`${API}/assets?limit=50`);
  expect(res.ok()).toBeTruthy();
  return (await res.json()) as AssetLite[];
}

async function listTags(request: APIRequestContext): Promise<TagLite[]> {
  const res = await request.get(`${API}/tags`);
  expect(res.ok()).toBeTruthy();
  return (await res.json()) as TagLite[];
}

async function createTag(request: APIRequestContext, name: string): Promise<string> {
  const res = await request.post(`${API}/tags`, { data: { name } });
  expect(res.ok()).toBeTruthy();
  return ((await res.json()) as TagLite).id;
}

async function tagAssets(request: APIRequestContext, assetIds: string[], tagId: string) {
  const res = await request.post(`${API}/batch/tag`, {
    data: { asset_ids: assetIds, tag_ids: [tagId] },
  });
  expect(res.ok()).toBeTruthy();
}

async function assetTagIds(request: APIRequestContext, id: string): Promise<string[]> {
  const res = await request.get(`${API}/assets/${id}/tags`);
  expect(res.ok()).toBeTruthy();
  return ((await res.json()) as TagLite[]).map((t) => t.id);
}

// Best-effort cleanup: detach the tag from the assets, then delete it.
async function cleanupTag(request: APIRequestContext, tagId: string, assetIds: string[]) {
  await request.post(`${API}/batch/untag`, { data: { asset_ids: assetIds, tag_id: tagId } });
  await request.delete(`${API}/tags/${tagId}`);
}

// Dispatches the app's tag→tag merge drop: fromId (carried on the drag) onto the
// sidebar node of toId, driving the real onDragOver/onDrop handlers.
async function dragTagOnto(page: Page, fromId: string, toId: string): Promise<void> {
  await page.evaluate(
    ({ fromId, toId }) => {
      const target = document.querySelector(`[data-tag-id="${toId}"]`);
      if (!target) throw new Error(`tag drop target missing: ${toId}`);
      const dt = new DataTransfer();
      dt.setData("application/x-hetu-tag", fromId);
      const init: DragEventInit = { bubbles: true, cancelable: true, dataTransfer: dt };
      target.dispatchEvent(new DragEvent("dragover", init));
      target.dispatchEvent(new DragEvent("drop", init));
    },
    { fromId, toId },
  );
}

test.describe("Tag merge & batch replace (issue #62)", () => {
  let assets: AssetLite[];

  test.beforeAll(async ({ request }) => {
    assets = await listAssets(request);
    expect(assets.length).toBeGreaterThanOrEqual(2);
  });

  test("drag one tag onto another merges globally, source tag disappears", async ({
    page,
    request,
  }) => {
    const stamp = Date.now();
    const fromId = await createTag(request, `合并源-${stamp}`);
    const toId = await createTag(request, `合并目标-${stamp}`);
    const tagged = [assets[0]!.id, assets[1]!.id];
    await tagAssets(request, tagged, fromId);

    try {
      await page.goto("/");
      await expect(page.getByTestId("grid-view")).toBeVisible();

      // Both tags render in the sidebar; drag source onto target.
      await expect(page.locator(`[data-tag-id="${fromId}"]`)).toBeVisible();
      await expect(page.locator(`[data-tag-id="${toId}"]`)).toBeVisible();
      await dragTagOnto(page, fromId, toId);

      // A confirmation gates the destructive merge; nothing happens until OK.
      await expect(page.getByTestId("tag-merge-confirm")).toBeVisible();
      await page.getByTestId("tag-merge-confirm-ok").click();

      // The source tag is gone globally.
      await expect
        .poll(() => listTags(request).then((ts) => ts.some((t) => t.id === fromId)))
        .toBe(false);
      // Every previously-tagged asset now carries the target tag, not the source.
      for (const id of tagged) {
        await expect
          .poll(() => assetTagIds(request, id))
          .toEqual([toId]);
      }
      // The source node vanished from the sidebar too.
      await expect(page.locator(`[data-tag-id="${fromId}"]`)).toHaveCount(0);
      await page.screenshot({ path: "e2e/screenshots/tag-merge.png" });
    } finally {
      await cleanupTag(request, toId, tagged);
    }
  });

  test("batch replace swaps a tag on selected assets only, source survives", async ({
    page,
    request,
  }) => {
    const stamp = Date.now();
    const fromId = await createTag(request, `替换源-${stamp}`);
    const toId = await createTag(request, `替换目标-${stamp}`);
    const selected = assets[0]!;
    const untouched = assets[1]!;
    const label = selected.display_name || selected.name;
    const otherLabel = untouched.display_name || untouched.name;
    expect(label, "two distinct assets are needed").not.toBe(otherLabel);
    // Both assets carry the source tag; only `selected` will be replaced.
    await tagAssets(request, [selected.id, untouched.id], fromId);

    try {
      await page.goto("/");
      await expect(page.getByTestId("grid-view")).toBeVisible();

      // Select just the one asset via its card's select checkbox.
      const card = page.getByTestId("asset-card").filter({ hasText: label });
      await card.hover();
      await card.getByTitle("选择").first().click();
      await expect(page.getByText(/已选/)).toBeVisible();

      // Open 替换标签, choose from→to, apply.
      await page.getByRole("button", { name: "替换标签" }).click();
      await expect(page.getByTestId("replace-tag-menu")).toBeVisible();
      await page.getByTestId("replace-tag-from").selectOption(fromId);
      await page.getByTestId("replace-tag-to").selectOption(toId);
      await page.getByTestId("replace-tag-apply").click();

      // The selected asset swapped source→target; the untouched asset kept source.
      await expect.poll(() => assetTagIds(request, selected.id)).toEqual([toId]);
      await expect.poll(() => assetTagIds(request, untouched.id)).toEqual([fromId]);
      // The source tag itself survives (the untouched asset still uses it).
      await expect
        .poll(() => listTags(request).then((ts) => ts.some((t) => t.id === fromId)))
        .toBe(true);
      await page.screenshot({ path: "e2e/screenshots/tag-batch-replace.png" });
    } finally {
      await cleanupTag(request, fromId, [selected.id, untouched.id]);
      await cleanupTag(request, toId, [selected.id, untouched.id]);
    }
  });
});
