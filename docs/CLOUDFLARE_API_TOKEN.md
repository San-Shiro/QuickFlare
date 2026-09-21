# How to Get a Cloudflare API Token for QuickFlare

This guide walks you through creating a scoped Cloudflare API token for QuickFlare. QuickFlare uses this token to discover your domains, create and manage your secure tunnels, and configure DNS records automatically.

---

## 1. Overview & Permissions

QuickFlare adheres to the principle of least privilege. It requires an **API Token** with only two specific permissions:

| Scope | Resource | Permission Level | Why QuickFlare Needs It |
|---|---|---|---|
| **Account** | **Cloudflare Tunnel** | **Edit** | Creates and configures the named tunnel, manages ingress rules, and fetches tunnel credentials. |
| **Zone** | **DNS** | **Edit** | Creates, updates, and deletes CNAME records (e.g. `app.yourdomain.com`) pointing to your tunnel. |

> [!IMPORTANT]
> **Do not use a Global API Key.** QuickFlare strictly requires a scoped API token. Scoped tokens are significantly safer because they can be restricted to specific permissions, specific domains, and revoked at any time without compromising your entire account.

---

## 2. Prerequisites

1. A [Cloudflare account](https://dash.cloudflare.com/sign-up) (the free tier is completely sufficient).
2. At least one domain added to your Cloudflare account with active status (nameservers pointing to Cloudflare).

---

## 3. Step-by-Step Instructions

### Step 1: Open the API Tokens Page
1. Log in to the [Cloudflare Dashboard](https://dash.cloudflare.com).
2. In the upper-right corner of the page, click your **User Profile** icon and select **My Profile**.
3. In the left navigation menu, click **API Tokens** (or navigate directly to [dash.cloudflare.com/profile/api-tokens](https://dash.cloudflare.com/profile/api-tokens)).

---

### Step 2: Start Custom Token Creation
1. Click the blue **Create Token** button.
2. Scroll to the bottom of the page to find the **Custom Token** section.
3. Click the **Get started** button next to *Create Custom Token*.

---

### Step 3: Set Token Name
Give your token a descriptive name so you remember what it is for:
- **Token name**: `QuickFlare` (or `QuickFlare-Desktop`)

---

### Step 4: Configure Permissions
In the **Permissions** section, set up the two required permissions:

1. **First Row (Account Level):**
   - Dropdown 1: **Account**
   - Dropdown 2: **Cloudflare Tunnel**
   - Dropdown 3: **Edit**

2. Click the **+ Add more** button directly below the first row to add a second permission row.

3. **Second Row (Zone Level):**
   - Dropdown 1: **Zone**
   - Dropdown 2: **DNS**
   - Dropdown 3: **Edit**

---

### Step 5: Configure Resources Scope
Specify which accounts and domains this token is allowed to access:

1. **Account Resources:**
   - Dropdown 1: **Include**
   - Dropdown 2: **All accounts** *(or select your specific account if you have access to multiple)*

2. **Zone Resources:**
   - Dropdown 1: **Include**
   - Dropdown 2: **All zones** *(recommended if you want to use QuickFlare across multiple domains)*  
     *Alternatively, select **Specific zone** and choose your specific domain.*

---

### Step 6: Client IP Filtering & TTL (Optional)
- **Client IP Address Filtering**: Leave blank unless you have a dedicated static IP and wish to restrict API access exclusively to that IP.
- **TTL (Start Date / End Date)**: Leave blank for perpetual use, or set an expiration date if you want the token to automatically expire.

---

### Step 7: Review and Create
1. Click **Continue to summary** at the bottom of the page.
2. Verify that your configuration matches:
   - **Account** → `Cloudflare Tunnel` → `Edit`
   - **Zone** → `DNS` → `Edit`
   - **Account Resources** → `All accounts` (or your account)
   - **Zone Resources** → `All zones` (or your selected zone)
3. Click **Create Token**.

---

### Step 8: Copy Your Token
1. Your newly minted API token will be displayed on the screen (a 40-character string).
2. Click the **Copy** button to copy the token to your clipboard.

> [!CAUTION]
> Cloudflare displays this token string **only once**. If you navigate away or close the page without copying it, you will need to roll or regenerate the token.

---

## 4. Adding the Token to QuickFlare

1. Click the **QuickFlare** icon in your Windows notification tray to open the application panel.
2. In the setup screen:
   - If this is your first time, paste the token into the **API Token** input field.
   - If QuickFlare is already configured, click the **Settings (gear)** icon and update the token field.
3. Click **Verify token**.
4. QuickFlare will query Cloudflare to validate your permissions and discover your domains.
5. Once verified, your domains will appear in the domain dropdown, and you can immediately begin creating routes.

---

## 5. Security & Token Storage

- **DPAPI Encryption**: When you save your token in QuickFlare, it is immediately encrypted using **Windows DPAPI** (Data Protection API). The cryptographic key is derived from your Windows user login credentials.
- **Encrypted at Rest**: The token is stored encrypted in `%APPDATA%\QuickFlare\config.json`. It is never stored in plaintext on disk.
- **Direct Communication**: QuickFlare communicates directly with `https://api.cloudflare.com` over HTTPS. The token is never shared with any third-party server or intermediary.

---

## 6. Troubleshooting

### "Invalid API Token" or "Verification failed"
- Ensure there are no leading or trailing whitespace characters or accidental line breaks when pasting the token.
- Verify that the token has not been revoked or expired in the Cloudflare dashboard.

### "No domains or zones found"
- Verify that the domain is listed under **Websites** in your Cloudflare dashboard with an **Active** status.
- Check the **Zone Resources** setting of your API token: if set to *Specific zone*, ensure the desired domain was selected.

### "Forbidden" or Cloudflare Error 10000
- Check that both permissions are set to **Edit** (not *Read*):
  - `Account` → `Cloudflare Tunnel` → `Edit`
  - `Zone` → `DNS` → `Edit`

### How to Revoke or Rotate the Token
1. Go to [dash.cloudflare.com/profile/api-tokens](https://dash.cloudflare.com/profile/api-tokens).
2. Locate your `QuickFlare` token.
3. Click the three dots (`...`) on the right:
   - Choose **Roll** to generate a new secret while keeping the same settings.
   - Choose **Revoke** to permanently disable the token. QuickFlare will immediately lose access.
