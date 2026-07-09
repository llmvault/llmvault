import { test } from './fixtures';
import { expect } from '@playwright/test';

test.describe('Authenticated navigation', () => {
  test('navigates to Agents page', async ({ authenticatedPage: page }) => {
    await page.getByRole('button', { name: 'Agents' }).first().click();
    await expect(page).toHaveURL(/\/w\/agents/);
    await expect(page.getByRole('heading', { name: /Agents/ })).toBeVisible();
  });

  test('navigates to Settings via account menu', async ({ authenticatedPage: page }) => {
    await page.getByRole('button', { name: 'Account and settings' }).click();
    await page.getByRole('button', { name: 'Settings' }).last().click();
    await expect(page).toHaveURL(/\/w\/settings/);
  });

  test('navigates to General settings', async ({ authenticatedPage: page }) => {
    await page.getByRole('button', { name: 'Account and settings' }).click();
    await page.getByRole('button', { name: 'Settings' }).last().click();
    await page.getByRole('link', { name: 'General' }).click();
    await expect(page).toHaveURL(/\/w\/settings\/general/);
    await expect(page.getByRole('heading', { name: 'General' })).toBeVisible();
  });

  test('navigates to Usage & billing', async ({ authenticatedPage: page }) => {
    await page.getByRole('button', { name: 'Account and settings' }).click();
    await page.getByRole('button', { name: 'Settings' }).last().click();
    await page.getByRole('link', { name: 'Usage & billing' }).click();
    await expect(page).toHaveURL(/\/w\/settings\/billing/);
    await expect(page.getByRole('heading', { name: /Billing|Usage/ }).first()).toBeVisible();
  });

  test('navigates to Teams settings', async ({ authenticatedPage: page }) => {
    await page.getByRole('button', { name: 'Account and settings' }).click();
    await page.getByRole('button', { name: 'Settings' }).last().click();
    await page.getByRole('link', { name: 'Teams' }).click();
    await expect(page).toHaveURL(/\/w\/settings\/teams/);
    await expect(page.getByRole('heading', { name: /Teams/ }).first()).toBeVisible();
  });

  test('navigates to Memories settings', async ({ authenticatedPage: page }) => {
    await page.getByRole('button', { name: 'Account and settings' }).click();
    await page.getByRole('button', { name: 'Settings' }).last().click();
    await page.getByRole('link', { name: 'Memories' }).click();
    await expect(page).toHaveURL(/\/w\/settings\/memories/);
    await expect(page.getByRole('heading', { name: /Memories/ }).first()).toBeVisible();
  });

  test('navigates to Knowledge settings', async ({ authenticatedPage: page }) => {
    await page.getByRole('button', { name: 'Account and settings' }).click();
    await page.getByRole('button', { name: 'Settings' }).last().click();
    await page.getByRole('link', { name: 'Knowledge' }).click();
    await expect(page).toHaveURL(/\/w\/settings\/knowledge/);
    await expect(page.getByRole('heading', { name: /Knowledge/ })).toBeVisible();
  });
});
