"use strict";

const test = require("node:test");
const assert = require("node:assert/strict");
const {
  normalizeDailyChallenge,
  dailyChallengePrompt
} = require("./editor.js");

test("normalizes the versioned daily challenge payload", () => {
  const input = {
    catalog_version: 1,
    date: "2026-07-28",
    slug: "tiny-robot",
    prompt_es: "Un robot diminuto",
    prompt_en: "A tiny robot"
  };
  assert.deepEqual(normalizeDailyChallenge(input), {
    catalogVersion: 1,
    date: "2026-07-28",
    slug: "tiny-robot",
    promptES: "Un robot diminuto",
    promptEN: "A tiny robot"
  });
});

test("rejects malformed or future challenge payloads", () => {
  const valid = {
    catalog_version: 1,
    date: "2026-07-28",
    slug: "tiny-robot",
    prompt_es: "Un robot diminuto",
    prompt_en: "A tiny robot"
  };
  assert.equal(normalizeDailyChallenge({ ...valid, catalog_version: 2 }), null);
  assert.equal(normalizeDailyChallenge({ ...valid, date: "28-07-2026" }), null);
  assert.equal(normalizeDailyChallenge({ ...valid, slug: "Tiny Robot" }), null);
  assert.equal(normalizeDailyChallenge({ ...valid, prompt_es: "<b>robot</b>" }), null);
  assert.equal(normalizeDailyChallenge({ ...valid, prompt_en: "" }), null);
});

test("selects the localized prompt without mutating the challenge", () => {
  const challenge = normalizeDailyChallenge({
    catalog_version: 1,
    date: "2026-07-28",
    slug: "tiny-robot",
    prompt_es: "Un robot diminuto",
    prompt_en: "A tiny robot"
  });
  assert.equal(dailyChallengePrompt(challenge, "es"), "Un robot diminuto");
  assert.equal(dailyChallengePrompt(challenge, "en"), "A tiny robot");
  assert.equal(dailyChallengePrompt(null, "es"), "");
});
