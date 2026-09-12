import { test, expect } from "@playwright/test";
import type { APIRequestContext, Page } from "@playwright/test";

// E2E for issue #62 (manual palette editing): the detail modal's palette editor
// deletes, adds, and recolors swatches — and, as a regression guard for #88, a
// swatch click in view mode still triggers a color search. To stay fully
// isolated (never disturbing palette.spec.ts's img-red-1/img-blue color-search
// fixtures), each test imports its OWN throwaway image, edits that, and trashes
// it afterward.

const API = "/api/dam";
const SHOT = process.env.SHOT_DIR ?? "test-results/palette-edit";

interface ApiSwatch {
  hex: string;
  weight: number;
}

async function getColors(request: APIRequestContext, id: string): Promise<ApiSwatch[]> {
  const res = await request.get(`${API}/assets/${id}/colors`);
  expect(res.ok()).toBeTruthy();
  return res.json();
}

// resetPalette forces a known palette: delete every swatch (each delete
// renumbers, so ord 0 is always a valid target) then add the given hexes.
async function resetPalette(request: APIRequestContext, id: string, hexes: string[]) {
  let colors = await getColors(request, id);
  while (colors.length > 0) {
    const res = await request.delete(`${API}/assets/${id}/colors/0`);
    expect(res.ok()).toBeTruthy();
    colors = await getColors(request, id);
  }
  for (const hex of hexes) {
    const res = await request.post(`${API}/assets/${id}/colors`, { data: { hex } });
    expect(res.ok()).toBeTruthy();
  }
}

// importFixture uploads a throwaway, uniquely-colored PNG through POST /import
// (generated in-page via canvas). The random color keeps each import's content
// hash unique so the dedup path never skips it. Returns the new asset's id+name.
async function importFixture(page: Page): Promise<{ id: string; name: string }> {
  const name = `palette-edit-fixture-${Date.now()}-${Math.floor(Math.random() * 1e6)}.png`;
  const color = `#${Math.floor(Math.random() * 0xffffff)
    .toString(16)
    .padStart(6, "0")}`;
  const id = await page.evaluate(
    async ({ name, color }) => {
      const c = document.createElement("canvas");
      c.width = 48;
      c.height = 48;
      const ctx = c.getContext("2d");
      if (!ctx) return "";
      ctx.fillStyle = color;
      ctx.fillRect(0, 0, 48, 48);
      const blob = await new Promise<Blob | null>((r) => c.toBlob(r, "image/png"));
      if (!blob) return "";
      const form = new FormData();
      form.append("file", blob, name);
      const res = await fetch("/api/dam/import", { method: "POST", body: form });
      const data = (await res.json()) as { asset?: { id: string } };
      return data.asset?.id ?? "";
    },
    { name, color },
  );
  return { id, name };
}

// Each test edits its own imported image, then trashes it, so the seed fixtures
// the other palette specs depend on are never disturbed.
let targetId = "";
let targetName = "";

test.beforeEach(async ({ page, request }) => {
  await page.goto("/");
  await expect(page.getByTestId("grid-view")).toBeVisible();
  const fx = await importFixture(page);
  expect(fx.id, "import fixture asset").toBeTruthy();
  targetId = fx.id;
  targetName = fx.name;
  await resetPalette(request, fx.id, ["#ff0000", "#00ff00", "#0000ff"]);
});

test.afterEach(async ({ request }) => {
  if (targetId) await request.post(`${API}/batch/trash`, { data: { asset_ids: [targetId] } });
});

const card = (page: Page, name: string) =>
  page.getByTestId("asset-card").filter({ hasText: name });

// The palette editor lives inside the detail modal overlay; scope to it so the
// inspector's read-only palette can never stand in for the modal's editor.
const editor = (page: Page) =>
  page.locator('[class*="overlay"]').getByTestId("palette-editor");

// openDetail opens the full-screen detail modal for the named asset. Pre-select
// with a single click so the dblclick that opens the modal is deterministic
// (see AssetCard's hover-preview note reused by palette.spec.ts).
async function openDetail(page: Page, name: string) {
  await page.goto("/");
  await expect(page.getByTestId("grid-view")).toBeVisible();
  const c = card(page, name).first();
  await c.click();
  await c.dblclick();
  await expect(page.locator('[class*="overlay"]')).toBeVisible({ timeout: 10_000 });
}

// pickColor drives a native <input type="color"> the way a real pick commits:
// set the value, then blur (the editor persists on blur, not on every drag).
async function pickColor(page: Page, testid: string, hex: string) {
  await editor(page)
    .getByTestId(testid)
    .first()
    .evaluate((el, val) => {
      const input = el as HTMLInputElement;
      input.focus();
      input.value = val;
      input.dispatchEvent(new Event("input", { bubbles: true }));
      input.dispatchEvent(new Event("change", { bubbles: true }));
      input.blur();
    }, hex);
}

test("delete a swatch shrinks the palette", async ({ page, request }) => {
  await openDetail(page, targetName);
  const ed = editor(page);
  await ed.getByTestId("palette-edit-toggle").click();

  const chips = ed.getByTestId("palette-color-input");
  await expect(chips).toHaveCount(3);
  await ed.getByTestId("palette-delete").first().click();
  await expect(chips).toHaveCount(2);

  // The deletion is server-persisted, not just optimistic UI state.
  await expect.poll(async () => (await getColors(request, targetId)).length).toBe(2);
});

test("add a swatch grows the palette and shows the new swatch", async ({ page, request }) => {
  await openDetail(page, targetName);
  const ed = editor(page);
  await ed.getByTestId("palette-edit-toggle").click();

  const chips = ed.getByTestId("palette-color-input");
  await expect(chips).toHaveCount(3);
  await pickColor(page, "palette-add-input", "#ffff00");
  await expect(chips).toHaveCount(4);

  // The new swatch is the persisted, visible last entry.
  await expect.poll(async () => (await getColors(request, targetId)).map((s) => s.hex)).toContain(
    "#ffff00",
  );
  await expect(ed.getByTestId("palette-color-input").nth(3)).toBeVisible();
  await page.screenshot({ path: `${SHOT}/added.png` });
});

test("adjusting a swatch changes its color", async ({ page, request }) => {
  await openDetail(page, targetName);
  const ed = editor(page);
  await ed.getByTestId("palette-edit-toggle").click();

  // Recolor the dominant swatch (ord 0) from red to magenta.
  await pickColor(page, "palette-color-input", "#ff00ff");
  await expect.poll(async () => (await getColors(request, targetId))[0]?.hex).toBe("#ff00ff");

  // The chip's rendered background tracks the new color once the PUT lands.
  await expect
    .poll(async () =>
      ed
        .locator('[class*="chipColor"]')
        .first()
        .evaluate((el) => getComputedStyle(el).backgroundColor),
    )
    .toBe("rgb(255, 0, 255)");
});

test("clicking a swatch body still triggers color search (#88 regression)", async ({ page }) => {
  await openDetail(page, targetName);
  const ed = editor(page);

  // View mode (edit toggle OFF by default): the strip's swatches search by color.
  const strip = ed.getByTestId("palette-strip");
  await expect(strip).toBeVisible();
  await strip.locator('[class*="paletteSwatch"]').first().click();

  // The modal closes and the grid switches to color-filter mode (ΔE badges),
  // exactly as before the editor existed — the search interaction is intact.
  await expect(page.locator('[class*="overlay"]')).toHaveCount(0, { timeout: 10_000 });
  await expect(page.locator('[class*="distance"]').first()).toBeVisible({ timeout: 10_000 });
});
