import { test, expect, type APIRequestContext } from "@playwright/test";

// Smoke test for Issue #48: the multi-page document pager + font download entry
// in the asset detail modal. Assertions read the real UI (double-click a grid
// card to open the modal, mirroring model-viewer.spec.ts) and the REST API
// (like collections.spec.ts). The library must be seeded from web/e2e/fixtures/
// with doc-multi.pdf (3 pages), doc-single.pdf (1 page) and specimen.ttf, and
// the backend needs a PDF renderer (pdftoppm/mutool) so the multi-page index
// actually exists — the beforeAll guards both prerequisites.

const API = "/api/dam";

interface AssetLite {
  id: string;
  kind: string;
  name: string;
}
interface PageLite {
  page_no: number;
  thumb_url: string;
}

async function listAssets(request: APIRequestContext): Promise<AssetLite[]> {
  const res = await request.get(`${API}/assets?limit=100`);
  expect(res.ok()).toBeTruthy();
  return (await res.json()) as AssetLite[];
}

const find = (assets: AssetLite[], needle: string): AssetLite => {
  const a = assets.find((x) => x.name.includes(needle));
  if (!a) throw new Error(`fixture asset "${needle}" not indexed — seed web/e2e/fixtures`);
  return a;
};

// Open a grid card's detail modal by name. A bare double-click races the
// batch-bar/inspector mount that fires on first selection — it can land
// between the two clicks and the browser drops the dblclick, or (in a larger
// library) the grid reflow after selection shifts a different card under the
// second click, opening the wrong asset (see palette.spec.ts's
// openDetailAndFilter for the same, already-documented race). Pre-selecting
// with a plain click first makes the follow-up dblclick land on the same,
// now-stable card.
async function openDetail(page: import("@playwright/test").Page, name: string) {
  const c = page.locator('[class*="card"]', { hasText: name }).first();
  await c.click();
  await expect(page.getByText("已选 1 项")).toBeVisible();
  await c.dblclick();
  await expect(page.locator('[class*="overlay"]')).toBeVisible();
}

test.describe("Document pager + font download (#48)", () => {
  let multi: AssetLite;
  let single: AssetLite;
  let font: AssetLite;

  test.beforeAll(async ({ request }) => {
    const assets = await listAssets(request);
    multi = find(assets, "doc-multi");
    single = find(assets, "doc-single");
    font = find(assets, "specimen");

    // The multi-page fixture must have a rendered per-page index, else the
    // renderer is missing and this whole suite is meaningless — fail loudly.
    const res = await request.get(`${API}/assets/${multi.id}/pages`);
    expect(res.ok()).toBeTruthy();
    const pages = (await res.json()) as PageLite[];
    expect(pages.length, "doc-multi.pdf should render 3 pages").toBe(3);

    // Single-page document exposes no per-page index (falls back to one image).
    const sres = await request.get(`${API}/assets/${single.id}/pages`);
    expect(sres.ok()).toBeTruthy();
    expect(((await sres.json()) as PageLite[]).length).toBeLessThanOrEqual(1);

    // The .ttf must be classified as a font so the font branch (not default) renders.
    expect(font.kind, "specimen.ttf should be kind=font").toBe("font");
  });

  test("multi-page: pager renders, buttons + thumbnails page, boundaries disable", async ({
    page,
  }) => {
    await page.goto("/");
    await openDetail(page, "doc-multi");

    const pager = page.getByTestId("document-pager");
    await expect(pager).toBeVisible();
    const counter = page.getByTestId("pager-counter");
    await expect(counter).toContainText("第 1 / 共 3 页");

    const img = page.getByTestId("pager-page-image");
    const page1Src = await img.getAttribute("src");
    expect(page1Src).toContain("/pages/1/thumb");

    // First page: "prev" disabled, "next" enabled.
    await expect(page.getByTestId("pager-prev")).toBeDisabled();
    await expect(page.getByTestId("pager-next")).toBeEnabled();

    // Next button advances the page: counter + image src both change.
    await page.getByTestId("pager-next").click();
    await expect(counter).toContainText("第 2 / 共 3 页");
    const page2Src = await img.getAttribute("src");
    expect(page2Src).not.toBe(page1Src);
    expect(page2Src).toContain("/pages/2/thumb");

    // Thumbnail strip jumps directly to a page.
    await page.locator('[data-testid="pager-thumb"][data-page="3"]').click();
    await expect(counter).toContainText("第 3 / 共 3 页");
    await expect(img).toHaveAttribute("src", /\/pages\/3\/thumb/);

    // Last page: "next" disabled, "prev" enabled.
    await expect(page.getByTestId("pager-next")).toBeDisabled();
    await expect(page.getByTestId("pager-prev")).toBeEnabled();
  });

  test("multi-page: ←/→ keys page the document without switching the underlying asset", async ({
    page,
  }) => {
    await page.goto("/");
    await openDetail(page, "doc-multi");

    const pager = page.getByTestId("document-pager");
    await expect(pager).toBeVisible();
    const counter = page.getByTestId("pager-counter");
    await expect(counter).toContainText("第 1 / 共 3 页");

    // At page 1, ArrowLeft is clamped (no wrap, no side effect).
    await pager.press("ArrowLeft");
    await expect(counter).toContainText("第 1 / 共 3 页");

    // ArrowRight pages forward. The modal title must stay on the SAME document —
    // proof the browse layout's window ←/→ listener did NOT also fire (the pager
    // stopPropagation'd the key), i.e. no double keyboard handling.
    const modalTitle = await page.locator('[class*="overlay"] h2').textContent();
    await pager.press("ArrowRight");
    await expect(counter).toContainText("第 2 / 共 3 页");
    await pager.press("ArrowRight");
    await expect(counter).toContainText("第 3 / 共 3 页");
    expect(await page.locator('[class*="overlay"] h2').textContent()).toBe(modalTitle);

    // ArrowRight at the last page is clamped.
    await pager.press("ArrowRight");
    await expect(counter).toContainText("第 3 / 共 3 页");

    // ArrowLeft pages back.
    await pager.press("ArrowLeft");
    await expect(counter).toContainText("第 2 / 共 3 页");
  });

  test("single-page document: no pager UI (regression)", async ({ page }) => {
    await page.goto("/");
    // openDetail already asserts the overlay opened; the single-page document
    // must show no paging chrome inside it.
    await openDetail(page, "doc-single");
    await expect(page.getByTestId("document-pager")).toHaveCount(0);
    await expect(page.getByTestId("pager-counter")).toHaveCount(0);
  });

  test("font asset: specimen + download-font button with the file endpoint href", async ({
    page,
  }) => {
    await page.goto("/");
    await openDetail(page, "specimen");

    const download = page.getByTestId("font-download");
    await expect(download).toBeVisible();
    await expect(download).toHaveText("下载字体文件");
    await expect(download).toHaveAttribute("href", new RegExp(`/assets/${font.id}/file$`));
    await expect(download).toHaveAttribute("download", "");
    // No paging chrome on a font asset.
    await expect(page.getByTestId("document-pager")).toHaveCount(0);
  });
});
