import { test as base, expect, type Page } from '@playwright/test';

export const test = base.extend<{ authenticatedPage: Page }>({
  authenticatedPage: async ({ page }, use) => {
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

    await use(page);
  },
});
