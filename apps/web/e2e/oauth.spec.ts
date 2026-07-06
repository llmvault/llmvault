import { test, expect } from '@playwright/test';

test.describe('Login page OAuth buttons', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/auth/login');
  });

  test('Continue with Google redirects to Google', async ({ page }) => {
    const googleButton = page.getByRole('button', { name: /Continue with Google/ });
    await expect(googleButton).toBeVisible();

    await googleButton.click();

    await expect(page).toHaveURL(/accounts\.google\.com/);
  });

  test('Continue with GitHub redirects to GitHub', async ({ page }) => {
    const githubButton = page.getByRole('button', { name: /Continue with GitHub/ });
    await expect(githubButton).toBeVisible();

    await githubButton.click();

    await expect(page).toHaveURL(/github\.com/);
  });
});

test.describe('Signup page OAuth buttons', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/auth/signup');
  });

  test('Continue with Google redirects to Google', async ({ page }) => {
    const googleButton = page.getByRole('button', { name: /Continue with Google/ });
    await expect(googleButton).toBeVisible();

    await googleButton.click();

    await expect(page).toHaveURL(/accounts\.google\.com/);
  });

  test('Continue with GitHub redirects to GitHub', async ({ page }) => {
    const githubButton = page.getByRole('button', { name: /Continue with GitHub/ });
    await expect(githubButton).toBeVisible();

    await githubButton.click();

    await expect(page).toHaveURL(/github\.com/);
  });
});
