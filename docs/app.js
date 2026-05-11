/**
 * app.js — Shared Agora + caption logic
 * Agora Translator Bot — GitHub Pages tester
 */

'use strict';

// ---------------------------------------------------------------------------
// Query-param parsing
// ---------------------------------------------------------------------------
function getParams() {
  const p = new URLSearchParams(window.location.search);
  return {
    appId:   p.get('appid')   || '',
    channel: p.get('channel') || 'translate-test',
    uid:     parseInt(p.get('uid'))  || null,   // null → Agora assigns
    bot:     parseInt(p.get('bot'))  || 2002,
    src:     p.get('src') || 'en',
    dst:     p.get('dst') || 'es',
  };
}

// ---------------------------------------------------------------------------
// CaptionManager
// ---------------------------------------------------------------------------
class CaptionManager {
  /**
   * @param {HTMLElement} srcEl  - element to render source-language captions into
   * @param {HTMLElement} dstEl  - element to render destination-language captions into
   */
  constructor(srcEl, dstEl) {
    this._srcEl = srcEl;
    this._dstEl = dstEl;
    // Tracks the current <span class="partial"> node per column
    this._srcPartial = null;
    this._dstPartial = null;
  }

  /**
   * Update captions.
   * @param {string}  lang      - ISO language code in the message (e.g. "en", "es")
   * @param {string}  text      - transcript text
   * @param {boolean} isFinal   - true = committed line, false = streaming partial
   * @param {string}  srcLang   - source language code (determines which column)
   * @param {string}  dstLang   - destination language code
   */
  update(lang, text, isFinal, srcLang, dstLang) {
    let el, partialKey;
    if (lang === srcLang) {
      el = this._srcEl;
      partialKey = '_srcPartial';
    } else if (lang === dstLang) {
      el = this._dstEl;
      partialKey = '_dstPartial';
    } else {
      // Unknown lang — put it in dst column as fallback
      el = this._dstEl;
      partialKey = '_dstPartial';
    }

    if (isFinal) {
      // Remove existing partial span for this column
      if (this[partialKey]) {
        this[partialKey].remove();
        this[partialKey] = null;
      }
      // Append a final line
      if (text.trim()) {
        const span = document.createElement('span');
        span.className = 'final-line';
        span.textContent = text;
        el.appendChild(span);
        el.scrollTop = el.scrollHeight;
      }
    } else {
      // Partial — update or create the partial span
      if (!this[partialKey]) {
        const span = document.createElement('span');
        span.className = 'partial';
        el.appendChild(span);
        this[partialKey] = span;
      }
      this[partialKey].textContent = text;
      el.scrollTop = el.scrollHeight;
    }
  }

  /** Clear all captions */
  clear() {
    this._srcEl.innerHTML = '';
    this._dstEl.innerHTML = '';
    this._srcPartial = null;
    this._dstPartial = null;
  }
}

// ---------------------------------------------------------------------------
// Stream-message handler
// ---------------------------------------------------------------------------
/**
 * Handle a raw stream-message payload from Agora data stream.
 * @param {number}         uid         - sender UID (bot UID expected)
 * @param {Uint8Array}     payload     - raw bytes
 * @param {CaptionManager} captionMgr
 * @param {string}         srcLang
 * @param {string}         dstLang
 */
function handleStreamMessage(uid, payload, captionMgr, srcLang, dstLang) {
  try {
    const text = new TextDecoder().decode(payload);
    const msg = JSON.parse(text);
    if (
      typeof msg.lang === 'string' &&
      typeof msg.text === 'string' &&
      typeof msg.isFinal === 'boolean'
    ) {
      captionMgr.update(msg.lang, msg.text, msg.isFinal, srcLang, dstLang);
    }
  } catch (e) {
    // Ignore malformed / non-JSON messages
  }
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/** Set a status badge element's text + class */
function setStatus(badgeEl, text, cls) {
  badgeEl.textContent = text;
  badgeEl.className = 'status-badge' + (cls ? ' ' + cls : '');
}

/** Log to a .status-log element */
function logStatus(logEl, msg, isError) {
  logEl.textContent = msg;
  logEl.className = 'status-log' + (isError ? ' error' : '');
}

/** Truncate app ID for display */
function truncateAppId(appId) {
  if (!appId) return '(not set)';
  if (appId.length <= 12) return appId;
  return appId.slice(0, 8) + '…' + appId.slice(-4);
}

/** Language code → human label */
function langLabel(code) {
  const map = {
    en: 'English', es: 'Spanish', fr: 'French', de: 'German',
    ja: 'Japanese', zh: 'Chinese', ko: 'Korean', pt: 'Portuguese',
    it: 'Italian', ru: 'Russian', ar: 'Arabic', hi: 'Hindi',
  };
  return map[code] || code.toUpperCase();
}
