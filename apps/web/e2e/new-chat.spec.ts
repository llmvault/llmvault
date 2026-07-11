import { test } from './fixtures';
import { expect } from '@playwright/test';

test.describe('New chat session', () => {
  // Live model streaming can take longer than the default 30s suite timeout.
  // Keep headroom above the longest individual wait (90s stream completion).
  test.setTimeout(210_000);

  test('creates a session, redirects, and streams an agent response', async ({
    authenticatedPage: page,
  }) => {
    const prompt = 'List all of your tools';

    // Start from the new-chat surface like a real user.
    await page.getByRole('button', { name: 'New chat' }).click();
    await page.waitForURL(/\/w\/?$/);

    // Scope the chat to an available team/channel via the composer pickers.
    await page.getByLabel('Select team').click();
    await page.getByRole('button', { name: 'QA and Testing', exact: true }).click();

    // Channel may already be preselected after team change; only pick if needed.
    const channelTrigger = page.getByLabel('Select channel');
    if (!(await channelTrigger.innerText()).match(/general/i)) {
      await channelTrigger.click();
      await page.getByLabel('Search channels').fill('general');
      await page.getByRole('button', { name: 'general', exact: true }).click();
    }

    // Keep the default Hivy agent; pin DeepSeek V4 Flash for this test.
    // Model selection does not close the picker — choosing a reasoning effort does.
    await page.getByLabel('Select model').click();
    const modelSearch = page.getByLabel('Search models');
    await expect(modelSearch).toBeVisible();
    await modelSearch.fill('DeepSeek V4 Flash');
    await page
      .getByRole('button', { name: 'DeepSeek V4 Flash', exact: true })
      .click();
    await page.getByRole('button', { name: 'Low', exact: true }).click();
    await expect(modelSearch).toBeHidden();

    const composer = page.getByPlaceholder(/Ask .+ to do something/);
    await composer.click();
    await composer.fill(prompt);
    await expect(page.getByRole('button', { name: 'Send' })).toBeEnabled();
    await page.getByRole('button', { name: 'Send' }).click();

    // Session creation redirects to /w/channels/{channel}/{sessionId}.
    await page.waitForURL(
      /\/w\/channels\/[^/]+\/[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}/i,
      { timeout: 30_000 },
    );

    // User message is visible on the session page.
    // The prompt also becomes the sidebar session title, so take the last match
    // (message bubble) rather than the sidebar button.
    await expect(page.getByText(prompt, { exact: true }).last()).toBeVisible({
      timeout: 15_000,
    });

    // Streaming starts (Stop replaces Send).
    await expect(page.getByRole('button', { name: 'Stop' })).toBeVisible({
      timeout: 30_000,
    });

    // Assistant output streams in; wait for completion signals rather than
    // exact model wording (which can vary slightly).
    await expect(page.getByRole('button', { name: 'Send' })).toBeVisible({
      timeout: 90_000,
    });
    await expect(
      page.getByRole('button', { name: 'Good response' }),
    ).toBeVisible({ timeout: 30_000 });

    // Follow-up composer is present on the session page after the first turn.
    await expect(
      page.getByPlaceholder('Ask for follow-up changes'),
    ).toBeVisible();
  });
});
