import { test, expect, type APIRequestContext } from "@playwright/test";

// Smoke test for Issue #62: the image-sequence step viewer in the asset detail
// modal. A run of consecutively-numbered images (seq-frame-01..04.png) is
// indexed as ONE sequence asset anchored at the lowest frame; a standalone image
// (seq-single.png) is not a sequence. The library must be seeded from
// web/e2e/fixtures/ with those files and scanned (bin/hetu scan) before the run;
// the beforeAll guards both prerequisites.

const API = "/api/dam";

interface AssetLite {
  id: string;
  kind: string;
  name: string;
}
interface FrameLite {
  frame_no: number;
  name: string;
}

async function listAssets(request: APIRequestContext): Promise<AssetLite[]> {
  const res = await request.get(`${API}/assets?limit=200`);
  expect(res.ok()).toBeTruthy();
  return (await res.json()) as AssetLite[];
}

const find = (assets: AssetLite[], needle: string): AssetLite => {
  const a = assets.find((x) => x.name.includes(needle));
  if (!a) throw new Error(`fixture asset "${needle}" not indexed — seed web/e2e/fixtures`);
  return a;
};

// Open a grid card's detail modal by name. Mirrors document-pager.spec's
// openDetail: pre-select with a plain click so the follow-up dblclick lands on
// the same, now-stable card (avoids the batch-bar mount race).
async function openDetail(page: import("@playwright/test").Page, name: string) {
  const c = page.locator('[class*="card"]', { hasText: name }).first();
  await c.click();
  await expect(page.getByText("已选 1 项")).toBeVisible();
  await c.dblclick();
  await expect(page.locator('[class*="overlay"]')).toBeVisible();
}

test.describe("Image sequence step viewer (#62)", () => {
  let seq: AssetLite;
  let single: AssetLite;

  test.beforeAll(async ({ request }) => {
    const assets = await listAssets(request);
    seq = find(assets, "seq-frame-01");
    single = find(assets, "seq-single");

    // The sequence anchor must expose a 4-frame index, else the numbered images
    // were indexed as separate assets and this suite is meaningless — fail loud.
    const res = await request.get(`${API}/assets/${seq.id}/frames`);
    expect(res.ok()).toBeTruthy();
    const frames = (await res.json()) as FrameLite[];
    expect(frames.length, "seq-frame-01..04 should index as 4 frames").toBe(4);

    // The four numbered images collapse into ONE asset (only the anchor).
    const anchors = assets.filter((a) => a.name.includes("seq-frame"));
    expect(anchors.length, "numbered frames collapse into one sequence asset").toBe(1);

    // The standalone image exposes no frame index (falls back to one image).
    const sres = await request.get(`${API}/assets/${single.id}/frames`);
    expect(sres.ok()).toBeTruthy();
    expect(((await sres.json()) as FrameLite[]).length).toBeLessThanOrEqual(1);
  });

  test("sequence: stepper renders, buttons step frames, boundaries disable", async ({ page }) => {
    await page.goto("/");
    await openDetail(page, "seq-frame-01");

    const viewer = page.getByTestId("sequence-viewer");
    await expect(viewer).toBeVisible();
    const counter = page.getByTestId("sequence-counter");
    await expect(counter).toContainText("1 / 4");

    const img = page.getByTestId("sequence-frame-image");
    const frame1Src = await img.getAttribute("src");
    expect(frame1Src).toContain("/frames/1");

    // First frame: "prev" disabled, "next" enabled.
    await expect(page.getByTestId("sequence-prev")).toBeDisabled();
    await expect(page.getByTestId("sequence-next")).toBeEnabled();

    // Next advances the frame: counter + image src both change.
    await page.getByTestId("sequence-next").click();
    await expect(counter).toContainText("2 / 4");
    const frame2Src = await img.getAttribute("src");
    expect(frame2Src).not.toBe(frame1Src);
    expect(frame2Src).toContain("/frames/2");

    // Step to the last frame: "next" disabled, "prev" enabled.
    await page.getByTestId("sequence-next").click();
    await page.getByTestId("sequence-next").click();
    await expect(counter).toContainText("4 / 4");
    await expect(img).toHaveAttribute("src", /\/frames\/4/);
    await expect(page.getByTestId("sequence-next")).toBeDisabled();
    await expect(page.getByTestId("sequence-prev")).toBeEnabled();
  });

  test("sequence: ←/→ keys step frames without switching the underlying asset", async ({ page }) => {
    await page.goto("/");
    await openDetail(page, "seq-frame-01");

    const viewer = page.getByTestId("sequence-viewer");
    await expect(viewer).toBeVisible();
    const counter = page.getByTestId("sequence-counter");
    await expect(counter).toContainText("1 / 4");

    // At frame 1, ArrowLeft is clamped (no wrap, no side effect).
    await viewer.press("ArrowLeft");
    await expect(counter).toContainText("1 / 4");

    // ArrowRight steps forward; the modal title stays on the SAME asset — proof
    // the browse layout's window ←/→ listener did not also fire.
    const modalTitle = await page.locator('[class*="overlay"] h2').textContent();
    await viewer.press("ArrowRight");
    await expect(counter).toContainText("2 / 4");
    await viewer.press("ArrowRight");
    await expect(counter).toContainText("3 / 4");
    expect(await page.locator('[class*="overlay"] h2').textContent()).toBe(modalTitle);

    // ArrowLeft steps back.
    await viewer.press("ArrowLeft");
    await expect(counter).toContainText("2 / 4");
  });

  test("standalone image: no stepper UI (regression)", async ({ page }) => {
    await page.goto("/");
    await openDetail(page, "seq-single");
    await expect(page.getByTestId("sequence-viewer")).toHaveCount(0);
    await expect(page.getByTestId("sequence-counter")).toHaveCount(0);
  });
});
