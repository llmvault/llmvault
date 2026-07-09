import { test, expect } from '@playwright/test';

test.describe('Email/password login', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/auth/login');
  });

  test('successful login with valid credentials lands on workspace', async ({ page }) => {
    const email = process.env.QA_EMAIL_LOGIN_PROD;
    const password = process.env.QA_PASSWORD_LOGIN_PROD;

    if (!email || !password) {
      throw new Error('QA_EMAIL_LOGIN_PROD and QA_PASSWORD_LOGIN_PROD must be set');
    }

    // Verify login form is visible
    await expect(page.getByRole('heading', { name: /Sign in to hivy/ })).toBeVisible();
    await expect(page.getByLabel('Work email')).toBeVisible();
    await expect(page.getByLabel('Password')).toBeVisible();
    await expect(page.getByRole('button', { name: 'Sign in' })).toBeVisible();

    // Fill credentials
    await page.getByLabel('Work email').fill(email);
    await page.getByLabel('Password').fill(password);

    // Submit
    await page.getByRole('button', { name: 'Sign in' }).click();

    // Wait for navigation to the workspace
    await page.waitForURL(/\/w/);
    await expect(page).toHaveURL(/\/w/);

    // Confirm we're in the authenticated workspace by checking the sidebar is present
    await expect(page.getByRole('button', { name: 'New chat' })).toBeVisible();
  });
});
