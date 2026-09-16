package api

import (
	"crypto/sha256"
	"crypto/subtle"
	"fmt"
	"net/http"
)

// This file is D-75/D-77's admin capability gate: the project's only server-side
// authorization. It wraps the embedded SPA handler for /admin and everything
// under it, so the admin document is unreachable without the capability cookie,
// and it renders its own offline page — no JavaScript, no external asset, no
// auth endpoint — when the key is missing or wrong (ADMN-04).

// The cookie's exact shape is D-75: HttpOnly (the client never reads the key),
// SameSite=Lax (a cross-site request cannot ride it), Path=/ (every admin path
// is covered), 12 hours (a show's length) — and deliberately NO Secure, because
// the venue LAN is plain http and a Secure cookie would never be sent.
const (
	// AdminCookieName is the capability cookie's name, and it is exported because
	// the WebSocket edge needs it too: D-76 reads the same cookie off the upgrade
	// request in internal/hub (the role only exists after the first frame, so the
	// join is where the check runs). One name, one place.
	AdminCookieName   = "auction_admin"
	adminCookieMaxAge = 43200 // 12h
	adminGatePath     = "/admin"
)

// Equal is the one comparison this phase has, and it is shared: the hub's admin
// join check (D-76) compares the same two values, so it must stay the single
// implementation. Both sides are hashed to a fixed width FIRST, which is what
// removes the length-dependent branch a raw variable-length comparison would
// expose (T-04-01); crypto/subtle then compares the digests in constant time.
//
// An empty supplied value is never authorized: a missing key must not match a
// missing expected key, and that fail-closed rule lives here rather than in each
// caller.
func Equal(supplied, expected string) bool {
	if supplied == "" {
		return false
	}
	suppliedDigest := sha256.Sum256([]byte(supplied))
	expectedDigest := sha256.Sum256([]byte(expected))
	return subtle.ConstantTimeCompare(suppliedDigest[:], expectedDigest[:]) == 1
}

// AdminGate returns the handler for GET /admin and the /admin/ subtree (both
// registered by cmd/server/main.go, ahead of the SPA fallback). The decision, in
// this order (D-75):
//
//   - a non-empty ?k= that matches → Set-Cookie + 303 to the literal /admin;
//   - a valid capability cookie → the wrapped SPA handler;
//   - otherwise → the gate page: 401 when a non-empty ?k= did not match, 200
//     when no key was supplied (an empty ?k= is "no key supplied").
//
// A valid cookie outranks a stale ?k=: the cookie is the capability, so an old
// bookmark cannot lock the presenter out mid-show.
func AdminGate(key string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var wrong bool
		if supplied := r.URL.Query().Get("k"); supplied != "" {
			if Equal(supplied, key) {
				http.SetCookie(w, &http.Cookie{
					Name:     AdminCookieName,
					Value:    supplied,
					Path:     "/",
					MaxAge:   adminCookieMaxAge,
					HttpOnly: true,
					SameSite: http.SameSiteLaxMode,
				})
				// The literal target, never anything the caller supplied (T-04-06).
				http.Redirect(w, r, adminGatePath, http.StatusSeeOther)
				return
			}
			// The wrong key is neither logged nor echoed anywhere: the document
			// below is compile-time constants only.
			wrong = true
		}

		if cookie, err := r.Cookie(AdminCookieName); err == nil && Equal(cookie.Value, key) {
			next.ServeHTTP(w, r)
			return
		}

		writeGatePage(w, wrong)
	})
}

// writeGatePage writes the offline document, the one surface in the project that
// is not React. Its cache directive is the same one the embedded index.html
// carries: a cached gate page after a key rotation would be a dead end (D-75).
func writeGatePage(w http.ResponseWriter, wrong bool) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", indexCacheControl)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if wrong {
		w.WriteHeader(http.StatusUnauthorized)
	}
	_, _ = w.Write([]byte(gateDocument(wrong)))
}

// The gate page's bilingual strings (UI-SPEC §Admin Gate Page): five constant
// pairs rendering six strings — the document <title> reuses the h1's pair. They
// are Go constants rather than dictionary keys because this page cannot reach
// the SPA bundle — the one sanctioned string set outside web/src/lib/i18n.ts. EN
// is the default and comes first; each VI run is marked lang="vi" so a screen
// reader pronounces it with the right voice.
const (
	gateTitleEN  = "Admin"
	gateTitleVI  = "Quản trị"
	gateNoteEN   = "Enter the admin key from the server log."
	gateNoteVI   = "Nhập khoá quản trị trong log máy chủ."
	gateErrorEN  = "Wrong key — check the /admin?k=… line in the server log and try again."
	gateErrorVI  = "Sai khoá — xem dòng /admin?k=… trong log máy chủ rồi thử lại."
	gateLabelEN  = "Admin key"
	gateLabelVI  = "Khoá quản trị"
	gateSubmitEN = "Open admin"
	gateSubmitVI = "Mở quản trị"
)

// gateBilingual renders one string in both languages: the EN run first (the
// default, in --foreground), the ` · ` separator, then the VI run in
// --muted-foreground (UI-SPEC §Admin Gate Page).
func gateBilingual(en, vi string) string {
	return `<span>` + en + `</span> · <span lang="vi" class="vi">` + vi + `</span>`
}

// gateDocumentHead is the document through the opening form tag. The inline
// style block is the token system written as literal hex, because the page must
// be perfect with the venue's internet off: no stylesheet link, no script tag,
// no webfont, no favicon, no image. Every percent sign stays literal here — this
// constant is not a format string.
const gateDocumentHead = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>` + gateTitleEN + ` · ` + gateTitleVI + `</title>
<style>
* { box-sizing: border-box; }
/* §Typography's two declared line heights, applied to this document too: the
   gate page is a row in the UI-SPEC's type table, and "Body 1.5 / heading 1.2
   apply to every text node in it" (UI-review finding 5). Body-level 1.5 is
   inherited by everything except the submit button, whose UA line-height would
   otherwise win — so the button states it again at its own Label role. */
body {
  margin: 0; background: #FFFBF0; color: #111111; line-height: 1.5;
  font-family: ui-sans-serif, system-ui, -apple-system, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif;
}
main { max-width: 720px; margin: 0 auto; padding: 48px 24px; }
form {
  display: flex; flex-direction: column; gap: 16px;
  background: #F2E9D5; border: 2px solid #111111; border-radius: 0;
  box-shadow: 4px 4px 0 0 #111111; padding: 24px;
}
h1 { margin: 0; font-size: 32px; font-weight: 700; line-height: 1.2; }
p { margin: 0; font-size: 14px; }
.vi { color: #5C5344; }
label { font-size: 14px; font-weight: 700; }
input {
  min-height: 44px; width: 100%; border: 2px solid #111111; background: #FFFFFF;
  font-size: 16px; padding: 8px; border-radius: 0; font-family: inherit;
}
input[aria-invalid="true"] { border-color: #E5484D; }
.error { border: 2px solid #E5484D; padding: 8px; font-size: 16px; color: #111111; }
button {
  min-height: 44px; padding: 0 16px; border: 2px solid #111111; background: #FF6B00;
  color: #111111; font-weight: 700; font-size: 14px; line-height: 1.5; border-radius: 0;
  box-shadow: 4px 4px 0 0 #111111; font-family: inherit; cursor: pointer;
  transition: transform 80ms ease, box-shadow 80ms ease;
}
button:active { transform: translate(2px, 2px); box-shadow: none; }
:focus-visible { outline: 2px solid #111111; outline-offset: 2px; }
@media (prefers-reduced-motion: reduce) { button { transition: none; } }
</style>
</head>
<body>
<main>
<form method="get" action="/admin">
`

// gateDocumentBody closes it out. It IS a format string: the %s placeholders
// carry the bilingual strings, the 401-only error line and the 401-only aria
// wiring on the field — and it carries no other percent sign by construction.
const gateDocumentBody = `<h1>%s</h1>
<p>%s</p>
%s<label for="k">%s</label>
<input id="k" name="k" type="text" autocomplete="off" spellcheck="false"%s>
<button type="submit">%s</button>
</form>
</main>
</body>
</html>
`

// gateDocument renders the gate page. wrong selects the 401 variant: the
// role="alert" error line plus aria-invalid/aria-describedby on the field.
// Nothing here interpolates request input — the field is never given a value and
// no supplied string is echoed — so the wrong key cannot appear in (or steer)
// the document (T-04-06).
func gateDocument(wrong bool) string {
	errorLine := ""
	invalid := ""
	if wrong {
		errorLine = `<p id="gate-error" class="error" role="alert">` +
			gateBilingual(gateErrorEN, gateErrorVI) + "</p>\n"
		invalid = ` aria-invalid="true" aria-describedby="gate-error"`
	}

	return gateDocumentHead + fmt.Sprintf(gateDocumentBody,
		gateBilingual(gateTitleEN, gateTitleVI),
		gateBilingual(gateNoteEN, gateNoteVI),
		errorLine,
		gateBilingual(gateLabelEN, gateLabelVI),
		invalid,
		gateBilingual(gateSubmitEN, gateSubmitVI),
	)
}
