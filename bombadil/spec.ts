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
// no page load answered with an HTTP error. Requests from a page are covered by noFailedRequests below.
export {
  noConsoleErrors,
  noHttpErrorCodes,
  noUncaughtExceptions,
  noUnhandledPromiseRejections,
} from "@antithesishq/bombadil/browser/defaults/properties";

// ---- Checks ----

const privacy = extract((state) => page.privacyProblems(state.window, state.document));

// Someone who is not signed in sees "This group is private." at /, and that page never shows a post, a
// comment, a member's name, or the group's name.
export const privatePageShowsNothing = always(() => privacy.current.length === 0);

const ownNew = extract((state) => page.ownNewProblems(state.document));

// Nothing a person wrote carries a "new" mark for them, post or comment.
export const ownWritingNeverNew = always(() => ownNew.current.length === 0);

const recorded = extract((state) => {
  const r = page.recorder(state.window);
  return {
    wrong: [...r.wrong, ...page.overdue(r, 10_000)],
    failed: [...r.failed, ...page.failedResources(state.window)],
    failing: r.failing,
    violations: r.violations,
    failureBarShown: page.isShown(state.window, state.document.getElementById("request-failed")),
  };
});

// What the page shows once an answer has come:
//   - After a post: one new post with the text as the server keeps it, by the person signed in, at the
//     top, shown as text, and an empty field.
//   - After a comment: one new comment with that text, by the person, shown as text, last in its post's
//     list, a count one higher, as many comments shown as the count says, and an empty field.
//   - After opening a post's comments: no new-comment mark on the post, as many comments shown as the
//     count says, and as many of them marked new as the count said were.
//   - After text that is refused: the message that says why, no new post at the top or a comment count
//     that has not changed, and the text still in its field.
//   - After "Earlier posts": more posts, or no button for more.
// Comparisons with what the page showed before are left out when another request from the same form
// or post was on its way, and on a page the browser restored from its back/forward cache. Every request
// is answered within ten seconds.
export const answersShowWhatWasDone = always(() => recorded.current.wrong.length === 0);

// No request from the page fails: Datastar reports no error, and nothing is answered with an HTTP error.
export const noFailedRequests = always(() => recorded.current.failed.length === 0);

// The failure bar shows exactly while a request has failed and no other has started since.
export const failureBarOnlyOnFailure = always(() => recorded.current.failureBarShown === recorded.current.failing);

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
// different with the given tag, so that the same text is not on a page twice. The second of each pair
// is how long to make the text with x's, so that the long ones are not spelled out in the log. A lone
// surrogate cannot pass through an action's arguments, which are JSON, so "{surrogate}" stands for one.
function texts(tag: string, maxLength: number): [string, number][] {
  const shapes: [string, number][] = [
    "Lunch at noon? " + tag,
    "<b>Not bold</b> & <script>alert('" + tag + "')</script> <img src=x onerror=alert(1)>",
    "First line " + tag + "\nSecond line\n\nAfter a blank line",
    "   \n\t  Space around " + tag + "  \n ",
    "Smørrebrød på taget " + tag + " 🥪 — naïve café, ﬁne, שלום, 日本語",
    "Zero\u{200b}width " + tag + "\u{2060}",
    "\u{feff}Byte order mark " + tag,
    "Lone surrogate {surrogate} " + tag,
    "**Not bold** _nor italic_ [a link](https://example.com/" + tag + ")",
    "",
    " \n\t \u{a0}\u{2003}",
    "\u{200b}\u{2060}",
    "Bell \u{7} " + tag,
    "Escape \u{1b}[31m red " + tag,
    "Nul \u{0} " + tag,
  ].map((text) => [text, 0]);
  return [...shapes, [tag, maxLength], [tag, maxLength + 1]];
}

// tag that makes a text different from those before it in the run.
const tag = (() => {
  let made = 0;
  return () => "#" + ++made;
})();

// write the given text into the form with the given ID, made as long as the given length with x's,
// and send it.
async function write(document: Document, formID: string, text: string, length: number) {
  const form = document.getElementById(formID);
  const field = form?.querySelector("textarea");
  const button = form?.querySelector<HTMLButtonElement>("button[type=submit]");
  if (!field || !button || button.disabled) return;
  text = text.replaceAll("{surrogate}", String.fromCharCode(0xd800));
  field.focus();
  field.value = text + "x".repeat(Math.max(0, length - text.length));
  field.dispatchEvent(new Event("input", { bubbles: true }));
  button.click();
}

const composerReady = extract((state) => {
  const button = state.document.querySelector<HTMLButtonElement>("#composer button[type=submit]");
  return button !== null && !button.disabled;
});

const writePost = registerCustomAction("writePost", async (document, _window, text: string, length: number) =>
  write(document, "composer", text, length),
);

const openThreads = extract((state) =>
  Array.from(state.document.querySelectorAll<HTMLDetailsElement>("details[open][id$='-thread']")).flatMap((details) => {
    const button = details.querySelector<HTMLButtonElement>("form[id$='-comment-form'] button[type=submit]");
    return button && !button.disabled ? [details.id.replace(/^post-/, "").replace(/-thread$/, "")] : [];
  }),
);

const writeComment = registerCustomAction("writeComment", async (document, _window, postID: string, text: string, length: number) =>
  write(document, "post-" + postID + "-comment-form", text, length),
);

const summaries = extract((state) =>
  Array.from(state.document.querySelectorAll("details[id$='-thread'] > summary")).flatMap((summary) => {
    if (!isVisible(state.window, summary)) return [];
    const point = clickablePoint(summary);
    if (!point || !inViewport(state.window, point)) return [];
    return [{ fingerprint: getFingerprint(summary), point }];
  }),
);

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

const onGroupPage = extract((state) => page.isGroupPage(state.document));

const signOut = registerCustomAction("signOut", async (document, window) => {
  page.forgetSignIn(window);
  const form = document.createElement("form");
  form.method = "post";
  form.action = "/logout";
  document.body.append(form);
  form.submit();
});

// Bombadil picks evenly between the action generators a specification exports, so this is the only one.
export const exploring = weighted([
  [100, clicks],
  [100, inputs],
  [50, scroll],
  [10, navigation],
  [1, waitOnce],
  // Opens or closes a post's comments. Bombadil's own clicks are on links, buttons, fields, and labels.
  [40, actions(() => summaries.current.map((target) => ({ Click: target })))],
  // Posts one of the shapes of text.
  [30, actions(() => (composerReady.current ? texts(tag(), page.postMaxLength).map(([text, length]) => writePost(text, length)) : []))],
  // Comments on a post whose comments are open, with one of the shapes of text.
  [30, actions(() => openThreads.current.flatMap((postID) => texts(tag(), page.commentMaxLength).map(([text, length]) => writeComment(postID, text, length))))],
  // Loads earlier posts. The button is at the end of the timeline, further down than scrolling often gets.
  [10, actions(() => (earlierOffered.current ? [loadEarlier()] : []))],
  // Signs out, by the route the app has for it, which no page links to yet.
  [2, actions(() => (onGroupPage.current ? [signOut()] : []))],
]);
