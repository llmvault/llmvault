import { test } from './fixtures';
import { expect } from '@playwright/test';

test.describe('Create channel', () => {
  test('creates a new channel in Sales & Marketing with Sales category', async ({ authenticatedPage: page }) => {
    const channelName = `e2e-sales-channel-${Date.now()}`;

    await page.getByRole('button', { name: 'Create channel in Sales & Marketing' }).click();
    await page.waitForURL(/\/w\/channels\/new/);
    await page.waitForLoadState('networkidle');

    await page.getByPlaceholder('Name this channel').fill(channelName);

    await page.getByLabel('Category').click();
    await page.getByRole('option', { name: 'Sales' }).click();

    await page.getByLabel('Select agent').click();
    await page.getByRole('button', { name: /Ricky - App builder/ }).click();

    await page.getByRole('button', { name: 'Create channel' }).last().click();

    await expect(page).toHaveURL(new RegExp(`/w/channels/${channelName}`));
  });
});
