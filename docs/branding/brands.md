# Third-party brand marks

Decision **ADR-0090** in Phoedox (project `phoedox`) says: a plugin for a
third-party product may show that product's own logo in the plugin store and in
the app **only** where the brand's own published guidelines allow such use for
third-party integrations or compatible products. Everywhere else the plugin uses
its generic icon tile (`tiles/<plugin>.svg`). The product name is always also
written in words, and the logo must never suggest endorsement.

This page records what each brand's own guidelines say. Only primary sources
were used (the brand's own site, legal pages or repository); logo sites, Simple
Icons and Wikipedia were not.

## Verdicts

| Verdict | Meaning | Plugin shows |
|---|---|---|
| **allowed** | The guidelines explicitly let third parties use the logo to indicate an integration or compatibility, without asking first. | the brand logo, under the conditions below |
| **ask** | The guidelines require written permission, a licence or a request first. | generic tile |
| **not allowed** | The guidelines forbid this use (for example logos only for press, text-only references, or "never as an app icon"). | generic tile |
| **unclear** | No explicit rule for third-party integrations was found. | generic tile |

Everything that is not **allowed** uses the tile. A verdict can change only when
the brand publishes new guidelines or grants written permission; record the
permission (date, contact, scope) here before switching a plugin to the logo.

**In use (2026-11-18):** GitHub (`logo.svg` black, `logo-dark.svg` white). Frigate
is allowed by its policy but waits for a confirmation, see its section;
Synology publishes no SVG.

Even where the logo is not allowed, the product name may be used in words to
describe compatibility (e.g. "for Proxmox VE", "works with Philips Hue") as long
as the brand's naming rules are followed.

## Overview

| Plugin | Product | Verdict | Conditions | Source (URL) | Checked |
|---|---|---|---|---|---|
| homeassistant | Home Assistant | ask | Logo is an Open Home Foundation trademark; commercial use (anything that markets a product, software or service for sale) needs written permission from partner@openhomefoundation.org. WWHA badges only for certified partners. | https://contributors.home-assistant.io/brand/name-and-logo/ ; https://github.com/OpenHomeFoundation/brand-assets | 2026-11-18 |
| hue | Philips Hue | not allowed | Developer terms 8(a)(xi) forbid Hue/Signify branding and logos; compatible apps may only use the words "works with Philips Hue" / "for Philips Hue" in product material, and must tell users the app is not a Signify product. | https://developers.meethue.com/?p=820 (Terms of Use – Developer Content) | 2026-11-18 |
| shelly | Shelly | unclear | No public brand/logo guideline for third parties found; "Works with Shelly" is a partner programme without published logo rules. | https://www.shelly.com/blogs/media-kit ; https://x.shelly.com/features | 2026-11-18 |
| tasmota | Tasmota | unclear | Logo files sit in the GPL-3.0 repo (`tools/logo/`), but no trademark or logo-use statement exists. | https://github.com/arendst/Tasmota (README, LICENSE.txt) | 2026-11-18 |
| esphome | ESPHome | ask | Same Open Home Foundation rule as Home Assistant: no commercial use without written permission (partner@openhomefoundation.org). | https://github.com/OpenHomeFoundation/brand-assets ; https://brands.openhomefoundation.org/esphome/ | 2026-11-18 |
| frigate | Frigate NVR | allowed | Integrations may use the logo **only to represent the Frigate node/service**, never implying the integration is Frigate or endorsed; generally not on own website/packaging without permission; don't alter. | https://github.com/blakeblackshear/frigate/blob/dev/TRADEMARK.md | 2026-11-18 |
| unifiprotect | UniFi Protect (Ubiquiti) | ask | No current public brand guideline; Ubiquiti terms grant no rights in its marks and require prior written permission to reproduce its graphics. | https://ui.com/legal/unifi/termsofservice | 2026-11-18 |
| fritzbox | FRITZ!Box (FRITZ! / AVM) | not allowed | Logo licence is for media reporting on FRITZ! products only (clause 1.2, 2.1, 3.1); no other use. | https://about.fritz.com/en/press/images-and-videos/license-agreement ; https://about.fritz.com/en/press/images-and-videos/terms-of-use | 2026-11-18 |
| mikrotik | MikroTik RouterOS | ask | "Many uses – including our logos, app and product icons … – will require a license first"; no compatibility exception; no sponsorship implication. | https://mikrotik.com/logo | 2026-11-18 |
| unifi | UniFi Network (Ubiquiti) | ask | As UniFi Protect. | https://ui.com/legal/unifi/termsofservice | 2026-11-18 |
| opnsense | OPNsense | unclear | Name and logo may promote "OPNsense based products or services" and "serve OPNsense … and their communities"; online use should link to opnsense.org. An integration in another product is not named explicitly. Official logo only as `.ai` (no SVG). Ask project@opnsense.org to confirm. | https://docs.opnsense.org/legal.html | 2026-11-18 |
| tailscale | Tailscale | unclear | Public media kit (logos + brand toolkit) but no statement on who may use the logo or for what. | https://tailscale.com/press ; https://tailscale.com/legal | 2026-11-18 |
| cloudflare | Cloudflare | ask | Word marks only; "Use of the Cloudflare logos (other than … Web Badges) requires our written permission." | https://www.cloudflare.com/trademark/ | 2026-11-18 |
| netbox | NetBox (NetBox Labs) | ask | Site terms §7: no use/display of NetBox name, trademark or logo without express written consent; no published trademark policy. | https://netboxlabs.com/terms-of-use/ | 2026-11-18 |
| proxmox | Proxmox VE | not allowed | "Don't use the Proxmox logo as a representation for your app's icon, your website, service, or your company." Permitted: linking to Proxmox, articles, less prominent than own brand. Name as "XY for Proxmox Virtual Environment". | https://www.proxmox.com/en/about/company-details/media-kit | 2026-11-18 |
| truenas | TrueNAS (iXsystems) | not allowed | Fair use "limited to text-only references … not logos"; compatibility only in text ("compatible with TrueNAS®"). | https://www.truenas.com/legal/trademarks/ | 2026-11-18 |
| synology | Synology DSM | allowed (no SVG) | Logo may show compatibility with Synology products; unmodified, clear space = the "S", no borders/effects/added text, no endorsement; trademark statement required. Official files are `.ai` + PNG only, so the tile stays until Synology provides an SVG. | https://www.synology.com/en-global/company/branding | 2026-11-18 |
| ups | Network UPS Tools (NUT) | unclear | GPL code; logo shown on the website, no trademark or logo-use statement. | https://networkupstools.org/ ; https://github.com/networkupstools/nut/blob/master/COPYING | 2026-11-18 |
| hetzner | Hetzner Cloud | ask | Press images/logos only "in the context with reference to Hetzner Online GmbH", editing needs written consent, "Usage must be communicated to press@hetzner.com"; no integration rule. | https://www.hetzner.com/pressroom/ | 2026-11-18 |
| kubernetes | Kubernetes (CNCF / Linux Foundation) | ask | LF policy: no LF logos on websites/marketing or in any commercial context without written permission; only logo links to the project are free. "Certified Kubernetes" marks only for conformant products. | https://www.linuxfoundation.org/legal/trademark-usage ; https://github.com/cncf/artwork (README, LICENSE.md) | 2026-11-18 |
| portainer | Portainer | unclear | No trademark or brand policy found on portainer.io or in the repo (zlib licence covers code only). | https://www.portainer.io ; https://github.com/portainer/portainer | 2026-11-18 |
| aws | AWS EC2 | not allowed | Fair use "in plain text only (no logos)"; logos only in materials approved in advance. Architecture Icons are licensed only for architecture diagrams and similar material, not as product icons. | https://aws.amazon.com/trademark-guidelines/ ; https://aws.amazon.com/architecture/icons/ | 2026-11-18 |
| azure | Microsoft Azure | not allowed | Logos and product icons (incl. Azure) need a licence; "Microsoft's logos, designs, and icons can never be used as your app icon". Compatibility may be stated in words. | https://www.microsoft.com/en-us/legal/intellectualproperty/trademarks | 2026-11-18 |
| m365 | Microsoft 365 / Intune | not allowed | As Azure. | https://www.microsoft.com/en-us/legal/intellectualproperty/trademarks | 2026-11-18 |
| gworkspace | Google Workspace | not allowed | "Don't use Google brand elements … as the logo for your application"; brand elements only for formal partners. Words "for"/"compatible with" + ™ are fine. | https://developers.google.com/workspace/marketplace/terms/branding ; https://about.google/brand-resource-center/guidance/partnerships | 2026-11-18 |
| checkmk | Checkmk | unclear | Public brand assets "as is"; combining them with other graphics needs prior written consent; no statement about integrations. | https://checkmk.com/company/brand-assets | 2026-11-18 |
| zabbix | Zabbix | unclear | Name may be used for plugins "… for Zabbix" and nominative fair use; logo use bound to the policy and Visual Identity Guidelines, no explicit integration permission; commercial use / "official link" needs a licence. | https://www.zabbix.com/trademark | 2026-11-18 |
| grafana | Grafana | ask | Only use for open source discussion/development/support of Grafana Labs' projects; "Any other use … requires a separate trademark license agreement". | https://grafana.com/trademark-policy/ | 2026-11-18 |
| uptimekuma | Uptime Kuma | unclear | MIT repo, no trademark or logo statement. | https://github.com/louislam/uptime-kuma | 2026-11-18 |
| github | GitHub | allowed | "Use a permitted GitHub logo to inform others that your project integrates with GitHub." Unmodified, black/white (rarely grey/green), keep clear space, never as own logo, no implication of being GitHub. | https://brand.github.com/foundations/logo | 2026-11-18 |
| gitlab | GitLab | not allowed | "Use of the Logos is not permitted under these Guidelines" (except unmodified CE distribution); requests for integration UIs "will generally be refused". Name rules for plugins: "… for GitLab" plus disclaimer. | https://handbook.gitlab.com/handbook/marketing/brand-and-product-marketing/brand/brand-activation/trademark-guidelines/ | 2026-11-18 |
| paperless | Paperless-ngx | unclear | GPL-3.0 repo, no trademark or logo statement. | https://github.com/paperless-ngx/paperless-ngx | 2026-11-18 |

Count: 3 allowed (one without usable SVG), 10 ask, 9 not allowed, 10 unclear (32 plugins).

## Allowed brands

### Frigate NVR (`frigate`)

Source: [TRADEMARK.md](https://github.com/blakeblackshear/frigate/blob/dev/TRADEMARK.md) (last updated November 2025), owner Frigate, Inc.

- Section 5: "If you are building a dashboard or integration that interfaces with
  Frigate, you may use the logo only to represent the Frigate node/service,
  provided it does not imply you *are* Frigate."
- Same section: "You generally cannot use the logo on your own website or product
  packaging without permission." The logo may therefore represent the Frigate
  plugin/service in the app and in the in-app plugin store; do **not** use it in
  marketing for Phoedox (website, ads, screenshots promoting Phoedox) without
  asking help@frigate.video.
- **Status in Phoedox (2026-11-18): not used yet.** store.phoedox.com is a public
  website, which the sentence above may cover, so the plugin keeps the tile
  until Frigate confirms the use (ask help@frigate.video). The files stay here
  for that case; the manifest does not name them.
- No implied affiliation, sponsorship or endorsement; do not alter the logo.
- "Frigate" must not be part of a commercial product name; the plugin name
  "Frigate" in the store is a compatibility reference ("… for Frigate NVR" style
  is explicitly allowed).
- No mandatory notice text. Show: "Frigate™, Frigate NVR™ and the Frigate logo
  are trademarks of Frigate, Inc. This plugin is not affiliated with or endorsed
  by Frigate, Inc."

### Synology DSM (`synology`)

Source: [Synology branding guide](https://www.synology.com/en-global/company/branding).

- Permitted: the "Synology" logo to market Synology products or to display
  "compatibility or optimization with Synology's products, technologies or
  services". The guide covers only the "Synology" logo (not DSM or other marks).
- Clear space equal to the "S" in the logo; no minimum size given.
- Preferred version: grey + black; all-grey, all-black or all-white versions on
  coloured backgrounds for contrast.
- Do not change opacity, rotate, flip, separate elements, add borders or effects,
  add text or marks, or alter it in any other way; no implication of Synology
  sponsorship, endorsement or approval. Synology may review and end any use.
- Required statement whenever the logo is used: "The "Synology" logo are
  trademarks of Synology, Inc., registered in the Republic of China (Taiwan) and
  other regions."
- **No file saved:** the official downloads are ZIPs with `.ai` and PNG only (no
  SVG). Per ADR-0090 we do not convert or redraw; the plugin keeps the tile until
  Synology (marketing@synology.com) provides an SVG.

### GitHub (`github`)

Source: [GitHub logo guidelines](https://brand.github.com/foundations/logo), files from https://brand.github.com/GitHub_Logos.zip.

- Permitted: "Use a permitted GitHub logo to inform others that your project
  integrates with GitHub" (also as a link to GitHub).
- Invertocat and wordmark only in white or black (in few cases grey or green);
  keep the clear space shown in the guide (the saved `_Clearspace` files include
  it); no minimum size given.
- Do not modify (colour, dimensions, …); do not suggest you are GitHub; never use
  a GitHub logo as the icon or logo of your own business/organisation or product.
- Anything else needs GitHub's prior written permission.
- Notice text: "GITHUB®, the GITHUB® logo design, the INVERTOCAT logo design,
  OCTOCAT®, and the OCTOCAT® logo design are trademarks of GitHub, Inc."

## Files downloaded

All files pass `pdk.CheckSVG` (phoedox-sdk `go/pdk/svg.go`): no script, image,
foreignObject, animation, event handlers, external references or DTD entities;
all under 64 KB. They are unchanged copies of the official files.

| File | Size | Official source |
|---|---|---|
| `plugins/frigate/logo.svg` | 933 B | https://raw.githubusercontent.com/blakeblackshear/frigate/dev/docs/static/img/branding/logo.svg (black bird, used by the Frigate docs site) |
| `plugins/frigate/logo-dark.svg` | 936 B | https://raw.githubusercontent.com/blakeblackshear/frigate/dev/docs/static/img/branding/logo-dark.svg (white bird for dark backgrounds) |
| `plugins/github/logo.svg` | 1313 B | `GitHub Logos/SVG/GitHub_Invertocat_Black_Clearspace.svg` from https://brand.github.com/GitHub_Logos.zip |
| `plugins/github/logo-dark.svg` | 1313 B | `GitHub Logos/SVG/GitHub_Invertocat_White_Clearspace.svg` from https://brand.github.com/GitHub_Logos.zip |

Not saved: Synology (no official SVG), OPNsense (official logo only as `.ai`;
verdict unclear anyway).
