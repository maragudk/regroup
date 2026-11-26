// The product's rules for the group page, as bombadil checks, and the actions bombadil explores it with.
// Run it with "make bombadil"; docs/research/bombadil.md says how, and how to read a failure.
//
// There are no roles yet. Every check holds for whoever is signed in, which "Switch person" in the
// preview build changes, and for someone who is not signed in.

import { always } from "@antithesishq/bombadil";
import { actions, extract, getFingerprint, registerCustomAction, weighted } from "@antithesishq/bombadil/browser";
import { clicks, inputs, navigation, scroll, waitOnce } from "@antithesishq/bombadil/browser/defaults/actions";
import { clickablePoint, inViewport, isVisible } from "@antithesishq/bombadil/browser/dom";

import * as page from "./page.ts";

// Bombadil's own checks: no uncaught exception or unhandled rejection, nothing logged as an error, and
// no page answered with an HTTP error. The last covers full page loads only; requests from the page
// are covered by noFailedRequests below.
export {
  noConsoleErrors,
  noHttpErrorCodes,
  noUncaughtExceptions,
  noUnhandledPromiseRejections,
} from "@antithesishq/bombadil/browser/defaults/properties";

// ---- Checks ----

const privacy = extract((state) => page.privacyProblems(state.window, state.document));

// Someone who is not signed in sees "This group is private." and never a post, a comment, a member's
// name, or the group's name.
export const privatePageShowsNothing = always(() => privacy.current.length === 0);

const ownNew = extract((state) => page.ownNewProblems(state.document));

// Nothing a person wrote carries a "new" mark for them, post or comment.
export const ownWritingNeverNew = always(() => ownNew.current.length === 0);

const recorded = extract((state) => {
  const r = page.recorder(state.window);
  return {
    wrong: [...r.wrong, ...page.overdue(r, 10_000)],
    failed: [...r.failed, ...page.failedResources(state.window)],
    violations: r.violations,
    failureBarShown: page.isShown(state.window, state.document.getElementById("request-failed")),
  };
});

// After someone posts, their post is at the top of the timeline, once, with exactly the text they
// wrote, shown as text, and the field is empty. After someone comments, the comment is last in that
// post's list, once, and the post's count has gone up by one. Opening a post's comments clears its
// new-comment mark, shows as many comments as the count says, and marks as many of them new as the
// count said were. What is refused says why, changes nothing, and stays in its field. Every request
// is answered within ten seconds.
export const answersShowWhatWasDone = always(() => recorded.current.wrong.length === 0);

// No request from the page fails: Datastar reports no error, and nothing is answered with an HTTP error.
export const noFailedRequests = always(() => recorded.current.failed.length === 0);

// The failure bar is never visible unless a request really failed.
export const failureBarOnlyOnFailure = always(() => !recorded.current.failureBarShown || recorded.current.failed.length > 0);

// The page breaks no rule of its content security policy.
export const noContentSecurityPolicyViolations = always(() => recorded.current.violations.length === 0);

const otherHosts = extract((state) => page.otherHostProblems(state.window, state.document));

// The page never asks any other host for anything.
export const noOtherHosts = always(() => otherHosts.current.length === 0);

const order = extract((state) => page.orderProblems(state.document));

// No day heading appears twice, days are in order, newest first, and posts within a day are newest
// first and from that day, also after "Earlier posts". At most one day is "Today" and one "Yesterday".
export const timelineInOrder = always(() => order.current.length === 0);

const duplicates = extract((state) => page.duplicateProblems(state.document));

// No post or comment appears twice on a page, and no element ID does.
export const nothingTwice = always(() => duplicates.current.length === 0);

const markup = extract((state) => page.textProblems(state.document));

// What people wrote is shown as text, never as markup.
export const writingShownAsText = always(() => markup.current.length === 0);

// ---- Actions ----

// What to write, in shapes that the rules for a body treat differently: text with markup, line breaks,
// whitespace around it, letters from beyond ASCII, invisible formatting, a lone surrogate, nothing
// but whitespace, control characters, and exactly as long as allowed and one longer. Each is made
// different with the given tag, so that the same text is seldom on a page twice. The second of each
// pair is how long to make the text with x's, so that the long ones are not spelled out in the log.
function texts(tag: string, maxLength: number): [string, number][] {
  const shapes: [string, number][] = [
    "Lunch at noon? " + tag,
    "<b>Not bold</b> & <script>alert('" + tag + "')</script> <img src=x onerror=alert(1)>",
    "First line " + tag + "\nSecond line\n\nAfter a blank line",
    "   \n\t  Space around " + tag + "  \n ",
    "Smørrebrød på taget " + tag + " 🥪 — naïve café, ﬁne, שלום, 日本語",
    "Zero\u{200b}width " + tag + "\u{2060}",
    "\u{feff}Byte order mark " + tag,
    "Lone surrogate \u{d800} " + tag,
    "**Not bold** _nor italic_ [a link](https://example.com/" + tag + ")",
    "",
    " \n\t \u{a0}\u{2003}",
    "\u{200b}\u{2060}",
    "Bell \u{7} " + tag,
    "Escape \u{1b}[31m red " + tag,
  ].map((text) => [text, 0]);
  return [...shapes, [tag, maxLength], [tag, maxLength + 1]];
}

// write the given text into the form with the given ID, made as long as the given length with x's,
// and send it.
async function write(document: Document, formID: string, text: string, length: number) {
  const form = document.getElementById(formID);
  const field = form?.querySelector("textarea");
  const button = form?.querySelector<HTMLButtonElement>("button[type=submit]");
  if (!field || !button || button.disabled) return;
  field.focus();
  field.value = text + "x".repeat(Math.max(0, length - text.length));
  field.dispatchEvent(new Event("input", { bubbles: true }));
  button.click();
}

let written = 0;

function tag(): string {
  written++;
  return "#" + written + "-" + (Date.now() % 1_000_000).toString(36);
}

const composerReady = extract((state) => {
  const button = state.document.querySelector<HTMLButtonElement>("#composer button[type=submit]");
  return button !== null && !button.disabled;
});

const writePost = registerCustomAction("writePost", async (document, _window, text: string, length: number) =>
  write(document, "composer", text, length),
);

// Posts one of the shapes of text.
const posting = actions(() => (composerReady.current ? texts(tag(), 10_000).map(([text, length]) => writePost(text, length)) : []));

const openThreads = extract((state) =>
  Array.from(state.document.querySelectorAll<HTMLDetailsElement>("details[open][id$='-thread']")).flatMap((details) => {
    const button = details.querySelector<HTMLButtonElement>("form[id$='-comment-form'] button[type=submit]");
    return button && !button.disabled ? [details.id.replace(/^post-/, "").replace(/-thread$/, "")] : [];
  }),
);

const writeComment = registerCustomAction("writeComment", async (document, _window, postID: string, text: string, length: number) =>
  write(document, "post-" + postID + "-comment-form", text, length),
);

// Comments on a post whose comments are open, with one of the shapes of text.
const commenting = actions(() =>
  openThreads.current.flatMap((postID) => texts(tag(), 2_000).map(([text, length]) => writeComment(postID, text, length))),
);

// Bombadil's clicks leave out a summary, which is what opens and closes a post's comments.
const summaries = extract((state) =>
  Array.from(state.document.querySelectorAll("details[id$='-thread'] > summary")).flatMap((summary) => {
    if (!isVisible(state.window, summary)) return [];
    const point = clickablePoint(summary);
    if (!point || !inViewport(state.window, point)) return [];
    return [{ fingerprint: getFingerprint(summary), point }];
  }),
);

// Opens or closes a post's comments.
const toggling = actions(() => summaries.current.map((target) => ({ Click: target })));

const earlierOffered = extract((state) => {
  const button = state.document.querySelector<HTMLButtonElement>("#earlier button[type=submit]");
  return button !== null && !button.disabled;
});

const loadEarlier = registerCustomAction("loadEarlier", async (document) => {
  const button = document.querySelector<HTMLButtonElement>("#earlier button[type=submit]");
  if (!button || button.disabled) return;
  button.scrollIntoView({ block: "center" });
  button.click();
});

// Loads earlier posts. The button is at the end of the timeline, further down than scrolling often gets.
const loadingEarlier = actions(() => (earlierOffered.current ? [loadEarlier()] : []));

const signedIn = extract((state) => page.isGroupPage(state.document));

const signOut = registerCustomAction("signOut", async (document, window) => {
  window.sessionStorage.removeItem(page.signedInKey);
  const form = document.createElement("form");
  form.method = "post";
  form.action = "/logout";
  document.body.append(form);
  form.submit();
});

// Signs out, by the route the app has for it, which no page links to yet.
const signingOut = actions(() => (signedIn.current ? [signOut()] : []));

// Only this generator is exported: bombadil picks evenly between the generators a specification
// exports, so the weights below would not count if the ones above were exported too.
export const exploring = weighted([
  [100, clicks],
  [100, inputs],
  [50, scroll],
  [10, navigation],
  [1, waitOnce],
  [40, toggling],
  [30, posting],
  [30, commenting],
  [10, loadingEarlier],
  [2, signingOut],
]);
