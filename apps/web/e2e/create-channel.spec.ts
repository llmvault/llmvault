import { test } from './fixtures';
import { expect } from '@playwright/test';

test.describe('Create channel', () => {
  test('creates a new channel in Sales & Marketing with Sales category', async ({ authenticatedPage: page }) => {
    const channelName = `e2e-sales-channel-${Date.now()}`;

    await page.goto('/w/channels/new');

    // Select team
    await page.getByLabel('Team').click();
    await page.getByRole('option', { name: 'Sales & Marketing' }).click();
    await page.waitForTimeout(500);

    // Fill channel name
    await page.getByPlaceholder('Name this channel').fill(channelName);

    // Select category
    await page.getByLabel('Category').click();
    await page.getByRole('option', { name: 'Sales' }).click();

    // Select agent (first available)
    await page.getByLabel('Select agent').click();
    await page.getByRole('button', { name: /Ricky - App builder Builds/ }).click();

    // Submit
    await page.locator('form button[type="submit"]').click();

    // Assert we land on the new channel page and the channel appears in the sidebar
    await expect(page).toHaveURL(new RegExp(`/w/channels/`));
    await expect(page.getByText(channelName)).toBeVisible();
  });
});
