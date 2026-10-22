Static landing page, served at the site host by the Caddy image built from
`deploy/landing/Dockerfile`. Only static files (HTML, CSS, JS, images) are
served; server side code (PHP) is not executed. Login flows that used the
mailcow API are replaced by the `/v0` API and the future console.
