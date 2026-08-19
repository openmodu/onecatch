import assert from "node:assert/strict";
import test from "node:test";
import {
  REVIEW_INSPECTOR_MAX_WIDTH,
  REVIEW_INSPECTOR_MIN_WIDTH,
  preferredReviewInspectorWidth,
} from "./reviewLayout.js";

test("review inspector remains readable on compact workbenches", () => {
  assert.equal(preferredReviewInspectorWidth(900), REVIEW_INSPECTOR_MIN_WIDTH);
  assert.equal(preferredReviewInspectorWidth(1280), 640);
  assert.equal(preferredReviewInspectorWidth(1440), 720);
});

test("review inspector does not consume ultra-wide workbenches", () => {
  assert.equal(preferredReviewInspectorWidth(1920), REVIEW_INSPECTOR_MAX_WIDTH);
  assert.equal(preferredReviewInspectorWidth(4632), REVIEW_INSPECTOR_MAX_WIDTH);
  assert.equal(preferredReviewInspectorWidth(5022), REVIEW_INSPECTOR_MAX_WIDTH);
});

test("review inspector falls back safely for invalid measurements", () => {
  assert.equal(preferredReviewInspectorWidth(undefined), REVIEW_INSPECTOR_MIN_WIDTH);
  assert.equal(preferredReviewInspectorWidth(Number.NaN), REVIEW_INSPECTOR_MIN_WIDTH);
});
