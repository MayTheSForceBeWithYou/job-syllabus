// A review term can legitimately contain "/" (e.g. "CI/CD", "SPIFFE/SPIRE")
// but a raw encodeURIComponent(term) still breaks POST /v1/reviews/{term}:
// AWS API Gateway HTTP APIs unconditionally URL-decode %2F back into a
// literal "/" before substituting the path into the backend integration
// (confirmed against a real request — the API returned "no route matches
// /v1/reviews/Ci/Cd Pipelines", a routing 404 from chi seeing an extra path
// segment, not the app's own "no pending review" 404). There's no API
// Gateway configuration that disables this. Base64url has no "/" or "+" in
// its alphabet, so it survives the trip intact regardless of what the term
// contains; internal/api/handlers_reviews.go decodes it back on arrival.
const BASE64_ALPHABET = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/';

function utf8Bytes(str: string): number[] {
  const bytes: number[] = [];
  for (const ch of str) {
    const code = ch.codePointAt(0)!;
    if (code < 0x80) {
      bytes.push(code);
    } else if (code < 0x800) {
      bytes.push(0xc0 | (code >> 6), 0x80 | (code & 0x3f));
    } else if (code < 0x10000) {
      bytes.push(0xe0 | (code >> 12), 0x80 | ((code >> 6) & 0x3f), 0x80 | (code & 0x3f));
    } else {
      bytes.push(
        0xf0 | (code >> 18),
        0x80 | ((code >> 12) & 0x3f),
        0x80 | ((code >> 6) & 0x3f),
        0x80 | (code & 0x3f),
      );
    }
  }
  return bytes;
}

// A dependency-free codec, not window.btoa/Buffer: btoa doesn't exist on
// React Native's native runtimes (only the web bundle has it), and Buffer
// isn't globally available without an extra polyfill dependency neither
// platform otherwise needs.
export function encodeTermParam(term: string): string {
  const bytes = utf8Bytes(term);
  let out = '';
  for (let i = 0; i < bytes.length; i += 3) {
    const b0 = bytes[i];
    const b1 = bytes[i + 1];
    const b2 = bytes[i + 2];
    out += BASE64_ALPHABET[b0 >> 2];
    out += BASE64_ALPHABET[((b0 & 0x03) << 4) | (b1 === undefined ? 0 : b1 >> 4)];
    out += b1 === undefined ? '' : BASE64_ALPHABET[((b1 & 0x0f) << 2) | (b2 === undefined ? 0 : b2 >> 6)];
    out += b2 === undefined ? '' : BASE64_ALPHABET[b2 & 0x3f];
  }
  return out.replace(/\+/g, '-').replace(/\//g, '_');
}
