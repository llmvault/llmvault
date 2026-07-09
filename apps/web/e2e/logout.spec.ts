import { test } from './fixtures';
import { expect } from '@playwright/test';


test.describe('Logout', () => {
  test('logging out redirects to the login page', async ({ authenticatedPage: page }) => {
    // Open the account menu
    await page.getByRole('button', { name: 'Account and settings' }).click();

    // Confirm the logout option is visible before clicking
    await expect(page.getByRole('button', { name: 'Log out' })).toBeVisible();

    // Perform logout
    await page.getByRole('button', { name: 'Log out' }).click();

    // Expect redirect back to the login page
    await expect(page).toHaveURL(/\/auth\/login/);

    // Confirm the login form is present after logout
    await expect(page.getByRole('heading', { name: /Sign in to hivy/ })).toBeVisible();
    await expect(page.getByLabel('Work email')).toBeVisible();
    await expect(page.getByLabel('Password')).toBeVisible();
  });
});
