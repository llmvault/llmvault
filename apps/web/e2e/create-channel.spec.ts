import { test } from './fixtures';
import { expect } from '@playwright/test';

test.describe('Create channel', () => {
  test('creates a new channel in Sales & Marketing with Sales category, then deletes it', async ({
    authenticatedPage: page,
  }) => {
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

    // Delete the channel to avoid leaving hanging data
    await page.goto('/w/settings/channels');
    await page.waitForLoadState('networkidle');

    await page.getByRole('link', { name: new RegExp(channelName) }).click();
    await page.waitForLoadState('networkidle');

    await page.getByRole('button', { name: 'Delete channel' }).click();
    await page.getByRole('alertdialog').getByRole('button', { name: 'Delete channel' }).click();

    await expect(page.getByText('Channel deleted')).toBeVisible();
    await expect(page).toHaveURL(/\/w\/settings\/channels/);
  });
});
