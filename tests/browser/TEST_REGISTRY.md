# Browser Test Registry

This document tracks all browser tests for the Wealth Tracker application.

## Status Legend
- ✅ Pass
- ❌ Fail
- ⬜ Not Run
- 🔄 Needs Re-test

---

## Iteration 1: Foundation

| ID | Test | Status | Last Run | Notes |
|----|------|--------|----------|-------|
| FOUND-01 | Server starts and responds | ✅ | 2025-12-09 | Health check returns `{"status":"ok"}` |
| FOUND-02 | Login page loads | ✅ | 2025-12-09 | Playwright MCP - dark theme renders correctly |
| FOUND-03 | Register page loads | ✅ | 2025-12-09 | Playwright MCP - form fields visible |
| FOUND-04 | Static CSS loads | ✅ | 2025-12-09 | Dark theme styling applied correctly |
| FOUND-05 | Static JS loads | ✅ | 2025-12-09 | Alpine.js store errors (non-blocking) |

---

## Iteration 2: Authentication

| ID | Test | Status | Last Run | Notes |
|----|------|--------|----------|-------|
| AUTH-01 | Register new user | ✅ | 2025-12-09 | Created "Browser Test User" - redirects to dashboard |
| AUTH-02 | Login existing user | ✅ | 2025-12-09 | Login after logout successful |
| AUTH-03 | Logout | ✅ | 2025-12-09 | Redirects to login page |
| AUTH-04 | Protected route redirect | ✅ | 2025-12-09 | Dashboard requires auth |
| AUTH-05 | Dark mode renders correctly | ✅ | 2025-12-09 | Luxury dark theme with amber accents |

---

## Iteration 3: Categories & Accounts

| ID | Test | Status | Last Run | Notes |
|----|------|--------|----------|-------|
| CAT-01 | Create category | ✅ | 2025-12-09 | Created "Investments" with chart-line icon |
| CAT-02 | Edit category | ⬜ | - | |
| CAT-03 | Delete category | ⬜ | - | |
| ACC-01 | Create account | ✅ | 2025-12-09 | Created "Nordnet Portfolio" linked to Investments |
| ACC-02 | Edit account | ⬜ | - | |
| ACC-03 | Link account to category | ✅ | 2025-12-09 | Category dropdown shows user's categories |

---

## Iteration 4: Transactions

| ID | Test | Status | Last Run | Notes |
|----|------|--------|----------|-------|
| TXN-01 | Add transaction | ⬜ | - | |
| TXN-02 | Balance calculation | ⬜ | - | |
| TXN-03 | Filter transactions | ⬜ | - | |

---

## Iteration 5: Dashboard & Charts

| ID | Test | Status | Last Run | Notes |
|----|------|--------|----------|-------|
| DASH-01 | Net worth display | ⬜ | - | |
| DASH-02 | Pie chart render | ⬜ | - | |
| DASH-03 | History chart render | ⬜ | - | |

---

## Iteration 6: Goals

| ID | Test | Status | Last Run | Notes |
|----|------|--------|----------|-------|
| GOAL-01 | Create goal | ⬜ | - | |
| GOAL-02 | Progress calculation | ⬜ | - | |

---

## Iteration 7: Multi-Currency

| ID | Test | Status | Last Run | Notes |
|----|------|--------|----------|-------|
| CURR-01 | Multi-currency conversion | ⬜ | - | |

---

## Iteration 8: Polish

| ID | Test | Status | Last Run | Notes |
|----|------|--------|----------|-------|
| THEME-01 | Dark/light toggle | ⬜ | - | |
| THEME-02 | Theme persistence | ⬜ | - | |
| RESP-01 | Mobile responsive | ⬜ | - | |
