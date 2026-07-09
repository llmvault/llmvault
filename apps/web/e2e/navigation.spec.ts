import { test, expect } from '@playwright/test';

test.describe('Authenticated navigation', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/auth/login');

    const email = process.env.QA_EMAIL_LOGIN_PROD;
    const password = process.env.QA_PASSWORD_LOGIN_PROD;

    if (!email || !password) {
      throw new Error('QA_EMAIL_LOGIN_PROD and QA_PASSWORD_LOGIN_PROD must be set');
    }

    await page.getByLabel('Work email').fill(email);
    await page.getByLabel('Password').fill(password);
    await page.getByRole('button', { name: 'Sign in' }).click();
    await page.waitForURL(/\/w/);
  });

  test('navigates to Agents page', async ({ page }) => {
    await page.getByRole('button', { name: 'Agents' }).first().click();
    await expect(page).toHaveURL(/\/w\/agents/);
    await expect(page.getByRole('heading', { name: /Agents/ })).toBeVisible();
  });

  test('navigates to Settings via account menu', async ({ page }) => {
    await page.getByRole('button', { name: 'Account and settings' }).click();
    await page.getByRole('button', { name: 'Settings' }).last().click();
    await expect(page).toHaveURL(/\/w\/settings/);
  });

  test('navigates to General settings', async ({ page }) => {
    await page.getByRole('button', { name: 'Account and settings' }).click();
    await page.getByRole('button', { name: 'Settings' }).last().click();
    await page.getByRole('link', { name: 'General' }).click();
    await expect(page).toHaveURL(/\/w\/settings\/general/);
    await expect(page.getByRole('heading', { name: 'General' })).toBeVisible();
  });

  test('navigates to Usage & billing', async ({ page }) => {
    await page.getByRole('button', { name: 'Account and settings' }).click();
    await page.getByRole('button', { name: 'Settings' }).last().click();
    await page.getByRole('link', { name: 'Usage & billing' }).click();
    await expect(page).toHaveURL(/\/w\/settings\/billing/);
    await expect(page.getByRole('heading', { name: /Billing|Usage/ }).first()).toBeVisible();
  });

  test('navigates to Teams settings', async ({ page }) => {
    await page.getByRole('button', { name: 'Account and settings' }).click();
    await page.getByRole('button', { name: 'Settings' }).last().click();
    await page.getByRole('link', { name: 'Teams' }).click();
    await expect(page).toHaveURL(/\/w\/settings\/teams/);
    await expect(page.getByRole('heading', { name: /Teams/ }).first()).toBeVisible();
  });

  test('navigates to Memories settings', async ({ page }) => {
    await page.getByRole('button', { name: 'Account and settings' }).click();
    await page.getByRole('button', { name: 'Settings' }).last().click();
    await page.getByRole('link', { name: 'Memories' }).click();
    await expect(page).toHaveURL(/\/w\/settings\/memories/);
    await expect(page.getByRole('heading', { name: /Memories/ })).toBeVisible();
  });

  test('navigates to Knowledge settings', async ({ page }) => {
    await page.getByRole('button', { name: 'Account and settings' }).click();
    await page.getByRole('button', { name: 'Settings' }).last().click();
    await page.getByRole('link', { name: 'Knowledge' }).click();
    await expect(page).toHaveURL(/\/w\/settings\/knowledge/);
    await expect(page.getByRole('heading', { name: /Knowledge/ })).toBeVisible();
  });
});
