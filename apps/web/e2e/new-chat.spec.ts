import { test } from './fixtures';
import { expect } from '@playwright/test';

const SESSION_URL =
  /\/w\/channels\/[^/]+\/[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}/i;

test.describe('New chat session', () => {
  test.setTimeout(210_000);

  test('creates a session, redirects, and streams an agent response', async ({
    authenticatedPage: page,
  }) => {
    const prompt = 'List all of your tools';
    const teamName = 'QA and Testing';
    const channelName = 'general';
    const modelName = 'DeepSeek V4 Flash';
    const reasoningEffort = 'Low';

    await page.getByRole('button', { name: 'New chat' }).click();
    await page.waitForURL(/\/w\/?$/);

    await page.getByLabel('Select team').click();
    await page.getByRole('button', { name: teamName, exact: true }).click();

    const channelTrigger = page.getByLabel('Select channel');
    if (!(await channelTrigger.innerText()).match(new RegExp(channelName, 'i'))) {
      await channelTrigger.click();
      await page.getByLabel('Search channels').fill(channelName);
      await page.getByRole('button', { name: channelName, exact: true }).click();
    }

    await page.getByLabel('Select model').click();
    const modelSearch = page.getByLabel('Search models');
    await expect(modelSearch).toBeVisible();
    await modelSearch.fill(modelName);
    await page.getByRole('button', { name: modelName, exact: true }).click();
    await page.getByRole('button', { name: reasoningEffort, exact: true }).click();
    await expect(modelSearch).toBeHidden();

    const composer = page.getByPlaceholder(/Ask .+ to do something/);
    await composer.click();
    await composer.fill(prompt);
    await expect(page.getByRole('button', { name: 'Send' })).toBeEnabled();
    await page.getByRole('button', { name: 'Send' }).click();

    await page.waitForURL(SESSION_URL, { timeout: 30_000 });

    const userMessage = page.getByText(prompt, { exact: true }).last();
    await expect(userMessage).toBeVisible({ timeout: 15_000 });

    await expect(page.getByRole('button', { name: 'Stop' })).toBeVisible({
      timeout: 30_000,
    });
    await expect(page.getByRole('button', { name: 'Send' })).toBeVisible({
      timeout: 90_000,
    });
    await expect(page.getByRole('button', { name: 'Good response' })).toBeVisible({
      timeout: 30_000,
    });

    await expect(page.getByPlaceholder('Ask for follow-up changes')).toBeVisible();
  });
});
