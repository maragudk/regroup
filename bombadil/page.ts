// What bombadil's checks of the group page read from it, and what they keep in the browser between
// states. Everything here runs in the page.
//
// The page has stable markers for what is read here (see steps 3 and 7 of
// docs/diary/2026-11-26-first-group-page.md), among them data-person on the name of the person signed
// in and data-author on the name of the author of a post or a comment. The label of a post's count of
// comments is still found by the page's structure, and the end of the timeline by its words.

// ---- What is on the page ----

export type PagePost = {
  id: string;
  author: string;
  body: string;
  // The body holds elements, so it was not shown as text.
  markup: boolean;
  // The post itself carries a "new" mark, as opposed to its count of comments.
  marked: boolean;
  datetime: string;
};

export type PageComment = {
  id: string;
  author: string;
  body: string;
  markup: boolean;
  marked: boolean;
};

export function isPrivatePage(document: Document): boolean {
  return document.querySelector("h1")?.textContent?.trim() === "This group is private.";
}

export function isGroupPage(document: Document): boolean {
  return document.getElementById("timeline") !== null && document.getElementById("composer") !== null;
}

// groupName on the sign of the group page.
export function groupName(document: Document): string | null {
  return isGroupPage(document) ? (document.querySelector("header h1")?.textContent?.trim() ?? null) : null;
}

// personName of whoever the group page is for, which the sign shows beside "Switch person".
export function personName(document: Document): string | null {
  if (!isGroupPage(document)) return null;
  return document.querySelector("[data-person]")?.textContent ?? null;
}

// own of the given post or comment: the first of its elements that match the given selector and are
// not part of a comment inside it.
function own(element: Element, selector: string): Element | null {
  return Array.from(element.querySelectorAll(selector)).find((e) => e.closest("[data-comment]") === (element.matches("[data-comment]") ? element : null)) ?? null;
}

function bodyOf(element: Element): { body: string; markup: boolean } {
  const body = own(element, "[data-body]");
  return { body: body?.textContent ?? "", markup: (body?.childElementCount ?? 0) > 0 };
}

export function posts(document: Document): PagePost[] {
  return Array.from(document.querySelectorAll("#timeline article[data-post]")).map((article) => ({
    id: article.getAttribute("data-post") ?? "",
    author: own(article, "[data-author]")?.textContent ?? "",
    ...bodyOf(article),
    marked: Array.from(article.querySelectorAll('[data-mark="new"]')).some((mark) => mark.closest("details") === null),
    datetime: article.querySelector("time")?.getAttribute("datetime") ?? "",
  }));
}

export function comments(document: Document, postID: string): PageComment[] {
  const list = document.getElementById("post-" + postID + "-comments");
  if (!list) return [];
  return Array.from(list.querySelectorAll("li[data-comment]")).map((item) => ({
    id: item.getAttribute("data-comment") ?? "",
    author: own(item, "[data-author]")?.textContent ?? "",
    ...bodyOf(item),
    marked: item.querySelector('[data-mark="new"]') !== null,
  }));
}

// summary of a post's comments: how many it says there are, and how many it says are new.
export function summary(document: Document, postID: string): { count: number | null; fresh: number } {
  const element = document.getElementById("post-" + postID + "-comments-summary");
  if (!element) return { count: null, fresh: 0 };
  const label = element.querySelector(":scope > span:first-child")?.textContent?.trim() ?? "";
  let count: number | null = null;
  if (label === "Comment") count = 0;
  else if (/^\d+ comments?$/.test(label)) count = parseInt(label, 10);
  const mark = element.querySelector('[data-mark="new"]')?.textContent?.trim() ?? "";
  return { count, fresh: /^\d+ new$/.test(mark) ? parseInt(mark, 10) : 0 };
}

export function isShown(window: Window, element: Element | null): boolean {
  if (!element) return false;
  const style = window.getComputedStyle(element);
  return style.display !== "none" && style.visibility !== "hidden";
}

// ---- What the server keeps of a body ----

// The longest post and comment, in characters, as model.PostBodyMaxLength and model.CommentBodyMaxLength.
export const postMaxLength = 10_000;
export const commentMaxLength = 2_000;

export type Cleaned = { body: string } | { refused: string };

// visible matches a character a reader can see, by the rule of model.visible: not white space, not
// default-ignorable, not another format character than a prepended concatenation mark, and not one of
// the few that fonts draw blank.
const visible = /[\u{600}-\u{605}\u{6DD}\u{70F}\u{890}\u{891}\u{8E2}\u{110BD}\u{110CD}]|[^\p{White_Space}\p{Default_Ignorable_Code_Point}\p{Cf}\u{2800}\u{1D159}\u{13441}\u{13442}\u{16FE4}]/u;

// clean a body by the rules of model.CleanPostBody and model.CleanCommentBody, as the browser sends it:
// whitespace is the Unicode White_Space property, control characters the Cc category, and a body with
// nothing [visible] in it is empty. The browser sends a lone surrogate as U+FFFD.
export function clean(raw: string, maxLength: number): Cleaned {
  let body = raw.toWellFormed().replace(/\r\n/g, "\n").replace(/\r/g, "\n");
  body = body.replace(/^\p{White_Space}+|\p{White_Space}+$/gu, "");
  if (!visible.test(body)) return { refused: "empty" };
  if (/\p{Cc}/u.test(body.replace(/[\n\t]/g, ""))) return { refused: "invalid" };
  if ([...body].length > maxLength) return { refused: "too long" };
  return { body };
}

// ---- What is kept in the browser ----

// The person is signed in, as far as this browser tab knows. Session storage outlives the page, but not
// the tab, and a run has one tab. It is set when the preview's form to sign in as someone is sent, and
// cleared when the page after signing out comes.
const signedInKey = "regroup-bombadil-signed-in";

// The form that signs out was sent, from the page with the time origin it holds. The page that sent it
// can still be read before the answer comes, and is still for someone signed in, so the next page is the
// first that is not.
const signingOutKey = "regroup-bombadil-signing-out";

// A form that signs in if its link works was sent: a sign-in link's, or an invitation's. Whether it did is
// known only from the page that answers it, which is the group page at / if it did, so the next page
// decides, and sets signedInKey if it did. It holds the time origin of the page that sent it, so that
// the page that sent it, which can still be read before the answer comes, decides nothing.
const signingInKey = "regroup-bombadil-signing-in";

// signOutSent, for when the form that signs out is sent from the given window's page.
export function signOutSent(window: Window) {
  session(window)?.setItem(signingOutKey, String(window.performance.timeOrigin));
}

// session storage of the given window, or null on a page that the browser makes itself, where no
// storage can be read, such as the one it shows on going back to the answer to a form, which it asks
// before sending again. Such a page shows nothing of the app.
function session(window: Window): Storage | null {
  try {
    return window.sessionStorage;
  } catch {
    return null;
  }
}

// Names of the group and of the people in it, as seen on pages for someone signed in, and on the
// preview's list of people. Local storage outlives the page, so the private page can be checked for them.
const namesKey = "regroup-bombadil-names";

export type Known = { groups: string[]; people: string[] };

export function known(window: Window): Known {
  try {
    return JSON.parse(window.localStorage.getItem(namesKey) ?? "") as Known;
  } catch {
    return { groups: [], people: [] };
  }
}

function learn(window: Window, groups: string[], people: string[]) {
  const k = known(window);
  const next = {
    groups: [...new Set([...k.groups, ...groups.filter((n) => n.trim() !== "")])],
    people: [...new Set([...k.people, ...people.filter((n) => n.trim() !== "")])],
  };
  if (next.groups.length !== k.groups.length || next.people.length !== k.people.length) {
    window.localStorage.setItem(namesKey, JSON.stringify(next));
  }
}

// A request the page sent, and what the page must show once its answer has come.
type Pending = {
  what: string;
  // ID of the element that sent it: the form, or the details of a post's comments.
  element: string;
  sent: number;
  started: boolean;
  // Something was typed into the form after it was sent.
  typed: boolean;
  // Another request from the same element was on its way at the same time, so their answers can show
  // each other's effects.
  overlapped: boolean;
  check: (p: Pending) => string[];
};

export type Recorder = {
  pending: Pending[];
  // Each answer whose page did not show what it should, with why.
  wrong: string[];
  // Each request that started while another from the same form or post's comments was on its way.
  twice: string[];
  // Each request that failed, as Datastar tells the page.
  failed: string[];
  // Each content security policy violation.
  violations: string[];
  // A request failed, and no other has started since. The page says so until one does.
  failing: boolean;
  // The page was restored from the browser's back/forward cache, so what it shows may be out of date.
  restored: boolean;
};

declare global {
  interface Window {
    regroupBombadil?: Recorder;
  }
}

// recorder of this page, set up on the first call. To see every request, it must be called before
// anything is done on the page.
export function recorder(window: Window): Recorder {
  if (window.regroupBombadil) return window.regroupBombadil;

  const document = window.document;
  const r: Recorder = { pending: [], wrong: [], twice: [], failed: [], violations: [], failing: false, restored: false };
  window.regroupBombadil = r;

  // The resource timings that the checks read stop at 250 by default.
  window.performance.setResourceTimingBufferSize(100_000);

  const expect = (what: string, element: string, check: (p: Pending) => string[]) => {
    const p: Pending = { what, element, sent: Date.now(), started: false, typed: false, overlapped: false, check };
    for (const other of r.pending) {
      if (other.element === element) other.overlapped = p.overlapped = true;
    }
    r.pending.push(p);
  };

  window.addEventListener("pageshow", (event) => {
    if (!event.persisted) return;
    r.restored = true;
    r.pending = [];
  });

  document.addEventListener(
    "submit",
    (event) => {
      const form = event.target;
      if (!(form instanceof HTMLFormElement)) return;

      const action = form.getAttribute("action") ?? "";
      if (action.endsWith("/preview/sign-in")) {
        window.sessionStorage.setItem(signedInKey, "1");
        return;
      }
      if (action.endsWith("/logout")) {
        signOutSent(window);
        return;
      }
      if (action.endsWith("/sign-in/link") || action.endsWith("/invitation")) {
        window.sessionStorage.setItem(signingInKey, String(window.performance.timeOrigin));
        return;
      }

      const raw = form.querySelector("textarea")?.value ?? "";
      const me = personName(document);

      if (form.id === "composer") {
        const cleaned = clean(raw, postMaxLength);
        const before = posts(document);
        expect("post " + JSON.stringify(raw.slice(0, 200)), form.id, (p) => checkPost(document, raw, cleaned, before, me, p.typed));
        return;
      }

      const comment = /^post-(.+)-comment-form$/.exec(form.id);
      if (comment) {
        const postID = comment[1]!;
        const cleaned = clean(raw, commentMaxLength);
        const before = comments(document, postID);
        const count = summary(document, postID).count;
        expect("comment " + JSON.stringify(raw.slice(0, 200)) + " on " + postID, form.id, (p) =>
          checkComment(document, postID, raw, cleaned, before, r.restored || p.overlapped ? null : count, me, p.typed),
        );
        return;
      }

      if (form.id === "earlier") {
        const before = posts(document).length;
        expect("earlier posts", form.id, () => {
          const after = posts(document).length;
          const more = document.querySelector("#earlier button") !== null;
          const ended = document.getElementById("earlier")?.textContent?.trim() === "Nothing was posted before this.";
          if (!more && !ended) return ["the timeline offers no earlier posts, and does not say that nothing was posted before"];
          return after > before || !more ? [] : ["the timeline still has " + after + " posts, and still offers earlier ones"];
        });
      }
    },
    true,
  );

  document.addEventListener(
    "toggle",
    (event) => {
      const details = event.target;
      if (!(details instanceof HTMLDetailsElement) || !details.open) return;
      const thread = /^post-(.+)-thread$/.exec(details.id);
      if (!thread) return;
      const postID = thread[1]!;
      // An opening already on its way is the only one: opening the comments again meanwhile sends
      // nothing. If it does send something, the recorder sees a second request start.
      if (r.pending.some((p) => p.element === details.id)) return;
      const fresh = summary(document, postID).fresh;
      // The count of new comments is compared only if nothing else could have changed it: no other
      // opening or comment on the way, and a page that was not restored, whose count may be out of date.
      expect("opening the comments of " + postID, details.id, (p) => {
        const commenting = r.pending.some((other) => other.element === "post-" + postID + "-comment-form");
        return checkOpen(document, postID, p.overlapped || commenting || r.restored ? null : fresh);
      });
    },
    true,
  );

  document.addEventListener(
    "input",
    (event) => {
      const form = (event.target as Element | null)?.closest?.("form");
      if (!form) return;
      for (const p of r.pending) if (p.element === form.id) p.typed = true;
    },
    true,
  );

  document.addEventListener("datastar-fetch", (event) => {
    const detail = (event as CustomEvent).detail as { type: string; el: Element | null; argsRaw: unknown };
    const element = detail.el?.id ?? "";
    switch (detail.type) {
      case "started": {
        r.failing = false;
        if (r.pending.some((p) => p.element === element && p.started)) r.twice.push(element + " sent a request while one of its own was on its way");
        const p = r.pending.find((p) => p.element === element && !p.started);
        if (p) p.started = true;
        break;
      }
      case "error":
      case "retries-failed": {
        r.failing = true;
        r.failed.push(element + ": " + detail.type + " " + JSON.stringify(detail.argsRaw ?? {}));
        // What the page shows after a failed request is the failure bar's business, not this answer's.
        const i = r.pending.findIndex((p) => p.element === element && p.started);
        if (i >= 0) r.pending.splice(i, 1);
        break;
      }
      case "finished": {
        const i = r.pending.findIndex((p) => p.element === element && p.started);
        if (i < 0) break;
        const [p] = r.pending.splice(i, 1);
        for (const problem of p!.check(p!)) r.wrong.push(p!.what + ": " + problem);
        break;
      }
    }
  });

  document.addEventListener("securitypolicyviolation", (event) => {
    const text = event.violatedDirective + " blocked " + (event.blockedURI || "inline");
    if (!r.violations.includes(text)) r.violations.push(text);
  });
  // Violations from before the recorder was set up, while the page loaded, come as buffered reports.
  if ("ReportingObserver" in window) {
    new window.ReportingObserver(
      (reports) => {
        for (const report of reports) {
          const body = report.body as unknown as { effectiveDirective?: string; blockedURL?: string };
          const text = body.effectiveDirective + " blocked " + (body.blockedURL || "inline");
          if (!r.violations.includes(text)) r.violations.push(text);
        }
      },
      { types: ["csp-violation"], buffered: true },
    ).observe();
  }

  return r;
}

// buttonProblems: the button of a form whose request is on its way is off, and so is the button of the
// comment form of a post whose comments are on their way.
export function buttonProblems(r: Recorder, document: Document): string[] {
  return r.pending.flatMap((p) => {
    const button = p.started ? document.getElementById(p.element)?.querySelector<HTMLButtonElement>("button[type=submit]") : null;
    return button && !button.disabled ? [p.element + " has its button on while its request is on its way"] : [];
  });
}

// overdue requests: sent more than the given time ago, and not answered.
export function overdue(r: Recorder, millis: number): string[] {
  const now = Date.now();
  return r.pending
    .filter((p) => now - p.sent > millis)
    .map((p) => p.what + ": " + (p.started ? "no answer after " : "not sent after ") + Math.round((now - p.sent) / 1000) + "s");
}

// ---- What an answer must have left on the page ----

// echoed text, as a refusal puts it back in its field: through HTML, which has no NUL.
function echoed(raw: string): string {
  return raw.toWellFormed().replace(/\0/g, "\u{fffd}");
}

// refusalProblems: the message beside the field says why the text was refused.
function refusalProblems(reason: string, message: string, kind: string): string[] {
  const expected: Record<string, string> = {
    empty: "Write something first.",
    "too long": "That is too long. A " + kind + " can be up to " + (kind === "post" ? postMaxLength : commentMaxLength).toLocaleString("en") + " characters.",
    invalid: "That text has characters that cannot be used here. Remove them and try again.",
  };
  return message === expected[reason] ? [] : ["expected it refused as " + reason + " with " + JSON.stringify(expected[reason]) + ", but the message is " + JSON.stringify(message)];
}

function checkPost(document: Document, raw: string, cleaned: Cleaned, before: PagePost[], me: string | null, typed: boolean): string[] {
  const problems: string[] = [];
  const after = posts(document);
  const field = document.querySelector<HTMLTextAreaElement>("#composer textarea");
  const message = document.querySelector("#composer [role=alert]")?.textContent?.trim() ?? "";

  if ("refused" in cleaned) {
    problems.push(...refusalProblems(cleaned.refused, message, "post"));
    if (after[0]?.id !== before[0]?.id) problems.push("expected it refused as " + cleaned.refused + ", but a post was added to the top");
    if (!typed && field?.value !== echoed(raw)) problems.push("the field no longer holds what was written, but " + JSON.stringify(field?.value?.slice(0, 200)));
    return problems;
  }

  const body = cleaned.body;
  const top = after[0];
  if (message !== "") problems.push("refused with " + JSON.stringify(message));
  if (!top) return [...problems, "the timeline is empty"];
  if (before.some((p) => p.id === top.id)) problems.push("the top post " + top.id + " was there before");
  if (top.body !== body) problems.push("the top post reads " + JSON.stringify(top.body.slice(0, 200)) + ", expected " + JSON.stringify(body.slice(0, 200)));
  if (top.markup) problems.push("the top post's text has elements in it");
  if (me !== null && top.author !== me) problems.push("the top post is by " + JSON.stringify(top.author) + ", expected " + JSON.stringify(me));
  const added = after.filter((p) => p.body === body && !before.some((b) => b.id === p.id)).length;
  if (added !== 1) problems.push(added + " new posts read so");
  if (!typed && field?.value !== "") problems.push("the field still holds " + JSON.stringify(field?.value?.slice(0, 200)));
  return problems;
}

function checkComment(
  document: Document,
  postID: string,
  raw: string,
  cleaned: Cleaned,
  before: PageComment[],
  // count the summary gave when the comment was sent, or null if another request may have changed it.
  count: number | null,
  me: string | null,
  typed: boolean,
): string[] {
  const problems: string[] = [];
  const after = comments(document, postID);
  const now = summary(document, postID).count;
  const form = document.getElementById("post-" + postID + "-comment-form");
  const field = form?.querySelector("textarea");
  const message = form?.querySelector("[role=alert]")?.textContent?.trim() ?? "";

  if ("refused" in cleaned) {
    problems.push(...refusalProblems(cleaned.refused, message, "comment"));
    if (count !== null && now !== count) problems.push("expected it refused as " + cleaned.refused + ", but the count went from " + count + " to " + now);
    if (!typed && field?.value !== echoed(raw)) problems.push("the field no longer holds what was written, but " + JSON.stringify(field?.value?.slice(0, 200)));
    return problems;
  }

  const body = cleaned.body;
  if (message !== "") problems.push("refused with " + JSON.stringify(message));
  const added = after.filter((c) => c.body === body && !before.some((b) => b.id === c.id));
  if (added.length !== 1) return [...problems, added.length + " new comments read " + JSON.stringify(body.slice(0, 200))];
  const comment = added[0]!;
  if (comment.markup) problems.push("the comment's text has elements in it");
  if (me !== null && comment.author !== me) problems.push("the comment is by " + JSON.stringify(comment.author) + ", expected " + JSON.stringify(me));
  if (count !== null) {
    if (after.at(-1)?.id !== comment.id) problems.push("the comment is not the last one");
    if (now !== count + 1) problems.push("the count went from " + count + " to " + now);
  }
  if (now !== after.length) problems.push("the count says " + now + ", but " + after.length + " comments are shown");
  if (!typed && field?.value !== "") problems.push("the field still holds " + JSON.stringify(field?.value?.slice(0, 200)));
  return problems;
}

// checkOpen of a post's comments. Fresh is how many the count said were new, or null if that cannot be
// compared.
function checkOpen(document: Document, postID: string, fresh: number | null): string[] {
  const problems: string[] = [];
  const { count, fresh: still } = summary(document, postID);
  const shown = comments(document, postID);
  if (still !== 0) problems.push("the count still says " + still + " new");
  if (document.querySelector("#post-" + CSS.escape(postID) + "-comments[data-loading]")) problems.push("the comments still say they are loading");
  if (count !== shown.length) problems.push("the count says " + count + ", but " + shown.length + " comments are shown");
  const marked = shown.filter((c) => c.marked).length;
  if (fresh !== null && marked !== fresh) problems.push("the count said " + fresh + " new, but " + marked + " comments are marked new");
  return problems;
}

// ---- Rules that hold on every page ----

// privacyProblems: someone not signed in gets the page that says the group is private at /, and that
// page shows nothing of the group: no post, comment, or text someone wrote, and no name seen on a page
// for someone signed in or on the preview's list of people. Names are matched as whole words, with
// their case. It learns the names as it goes.
export function privacyProblems(window: Window, document: Document): string[] {
  const storage = session(window);
  if (storage === null) return [];
  const path = window.location.pathname;
  const signingOut = storage.getItem(signingOutKey);
  if (signingOut !== null && signingOut !== String(window.performance.timeOrigin)) {
    for (const key of [signingOutKey, signingInKey, signedInKey]) storage.removeItem(key);
  }
  const signingIn = storage.getItem(signingInKey);
  if (signingIn !== null && signingIn !== String(window.performance.timeOrigin)) {
    storage.removeItem(signingInKey);
    if (path === "/" && isGroupPage(document)) storage.setItem(signedInKey, "1");
  }
  const signedIn = storage.getItem(signedInKey) === "1";

  if (isGroupPage(document) && signedIn) {
    const people = [personName(document) ?? ""];
    for (const p of posts(document)) people.push(p.author, ...comments(document, p.id).map((c) => c.author));
    learn(window, [groupName(document) ?? ""], people);
  }
  const listed = Array.from(document.querySelectorAll('form[action$="/preview/sign-in"] button')).map((b) => b.textContent ?? "");
  if (listed.length > 0) learn(window, [], listed);

  const isPrivate = isPrivatePage(document);
  const problems: string[] = [];
  if (!signedIn && path === "/" && !isPrivate) problems.push("someone not signed in got a page at / that does not say the group is private");
  if (!isPrivate && (signedIn || path !== "/")) return problems;

  const count = (selector: string) => document.querySelectorAll(selector).length;
  if (count("article, [data-post]") > 0) problems.push("it shows " + count("article, [data-post]") + " posts");
  if (count("[data-comment]") > 0) problems.push("it shows " + count("[data-comment]") + " comments");
  if (count("[data-body]") > 0) problems.push("it shows " + count("[data-body]") + " texts that someone wrote");

  const text = (document.body?.innerText ?? "") + "\n" + document.title;
  const k = known(window);
  for (const name of [...k.groups, ...k.people]) {
    const escaped = name.replace(/[.*+?^$(){}|[\]\\]/g, "\\$&");
    if (new RegExp("(?<![\\p{L}\\p{N}])" + escaped + "(?![\\p{L}\\p{N}])", "u").test(text)) problems.push("it names " + JSON.stringify(name));
  }
  return problems;
}

// ownNewProblems: nothing a person wrote carries a "new" mark for them. The names are found by their
// markers, and a name that cannot be found is a problem too, rather than a check that quietly passes.
export function ownNewProblems(document: Document): string[] {
  if (!isGroupPage(document)) return [];
  const me = personName(document);
  if (!me) return ["the name of the person signed in is not where the checks look for it"];
  const problems: string[] = [];
  for (const p of posts(document)) {
    if (!p.author) problems.push("the author of post " + p.id + " is not where the checks look for it");
    if (p.author === me && p.marked) problems.push("post " + p.id + " by " + me + " is marked new for them");
    for (const c of comments(document, p.id)) {
      if (!c.author) problems.push("the author of comment " + c.id + " is not where the checks look for it");
      if (c.author === me && c.marked) problems.push("comment " + c.id + " by " + me + " is marked new for them");
    }
  }
  return problems;
}

// orderProblems: days are in order, newest first, each once, and so are the posts in a day.
export function orderProblems(document: Document): string[] {
  const dayNumber = (date: string) => Date.UTC(+date.slice(0, 4), +date.slice(5, 7) - 1, +date.slice(8, 10)) / 86_400_000;
  const problems: string[] = [];
  const sections = Array.from(document.querySelectorAll("#timeline section[data-day]"));
  const days = sections.map((s) => s.getAttribute("data-day") ?? "");
  for (let i = 1; i < days.length; i++) {
    if (!(days[i - 1]! > days[i]!)) problems.push("day " + days[i] + " comes after day " + days[i - 1]);
  }

  const headings = sections.map((s) => s.querySelector("h2")?.textContent ?? "");
  const today = headings.flatMap((h, i) => (h.startsWith("Today") ? [i] : []));
  const yesterday = headings.flatMap((h, i) => (h.startsWith("Yesterday") ? [i] : []));
  if (today.length > 1) problems.push(today.length + " days are headed Today: " + today.map((i) => days[i]).join(", "));
  if (yesterday.length > 1) problems.push(yesterday.length + " days are headed Yesterday: " + yesterday.map((i) => days[i]).join(", "));
  if (today.length === 1 && today[0] !== 0) problems.push("Today, " + days[today[0]!] + ", is not the first day");
  if (today.length === 1 && yesterday.length === 1 && dayNumber(days[yesterday[0]!]!) !== dayNumber(days[today[0]!]!) - 1) {
    problems.push("Yesterday, " + days[yesterday[0]!] + ", is not the day before Today, " + days[today[0]!]);
  }

  sections.forEach((section, i) => {
    let previous = Infinity;
    for (const article of Array.from(section.querySelectorAll("article[data-post]"))) {
      const datetime = article.querySelector("time")?.getAttribute("datetime") ?? "";
      const id = article.getAttribute("data-post");
      if (datetime.slice(0, 10) !== days[i]) problems.push("post " + id + " from " + datetime + " is under day " + days[i]);
      const t = Date.parse(datetime);
      if (t > previous) problems.push("post " + id + " from " + datetime + " comes after an older post on " + days[i]);
      previous = t;
    }
  });
  return problems;
}

// duplicateProblems: no post, comment, or element ID is on the page twice.
export function duplicateProblems(document: Document): string[] {
  const twice = (values: string[]) => [...new Set(values.filter((v, i) => values.indexOf(v) !== i))];
  const all = (selector: string, value: (e: Element) => string) => Array.from(document.querySelectorAll(selector)).map(value);
  return [
    ...twice(all("[data-post]", (e) => e.getAttribute("data-post") ?? "")).map((id) => "post " + id + " is shown more than once"),
    ...twice(all("[data-comment]", (e) => e.getAttribute("data-comment") ?? "")).map((id) => "comment " + id + " is shown more than once"),
    ...twice(all("[id]", (e) => e.id)).map((id) => "the ID " + id + " is on more than one element"),
  ];
}

// textProblems: what people wrote is shown as text, never as markup.
export function textProblems(document: Document): string[] {
  return Array.from(document.querySelectorAll("[data-body]"))
    .filter((e) => e.childElementCount > 0)
    .map((e) => "the text " + JSON.stringify(e.innerHTML.slice(0, 200)) + " has elements in it");
}

// otherHostProblems: the page asks no other host for anything, and names none to load anything from.
export function otherHostProblems(window: Window, document: Document): string[] {
  const origin = window.location.origin;
  const problems: string[] = [];
  const foreign = (url: string | null | undefined, base: string = window.location.href) => {
    if (!url) return false;
    try {
      const u = new URL(url, base);
      return (u.protocol === "http:" || u.protocol === "https:") && u.origin !== origin;
    } catch {
      return false;
    }
  };

  for (const entry of window.performance.getEntriesByType("resource")) {
    if (foreign(entry.name)) problems.push("it asked for " + entry.name);
  }

  const attributes: [string, string][] = [
    ["script[src]", "src"], ["link[href]", "href"], ["img[src]", "src"], ["iframe[src]", "src"], ["source[src]", "src"],
    ["video[src]", "src"], ["video[poster]", "poster"], ["audio[src]", "src"], ["embed[src]", "src"], ["object[data]", "data"],
    ["form[action]", "action"], ["button[formaction]", "formaction"], ["input[formaction]", "formaction"],
  ];
  for (const [selector, attribute] of attributes) {
    for (const element of Array.from(document.querySelectorAll(selector))) {
      const value = element.getAttribute(attribute);
      if (foreign(value)) problems.push("<" + element.tagName.toLowerCase() + " " + attribute + "=" + JSON.stringify(value) + ">");
    }
  }
  for (const element of Array.from(document.querySelectorAll("img[srcset], source[srcset]"))) {
    for (const candidate of (element.getAttribute("srcset") ?? "").split(",")) {
      const url = candidate.trim().split(/\s+/)[0];
      if (foreign(url)) problems.push("<" + element.tagName.toLowerCase() + " srcset> names " + url);
    }
  }

  const urls = (css: string) => Array.from(css.matchAll(/url\(\s*["']?([^"')]+)["']?\s*\)|@import\s+["']([^"']+)["']/g)).map((m) => m[1] ?? m[2] ?? "");
  for (const element of Array.from(document.querySelectorAll("[style]"))) {
    for (const url of urls(element.getAttribute("style") ?? "")) if (foreign(url)) problems.push("a style attribute names " + url);
  }
  for (const sheet of Array.from(document.styleSheets)) {
    if (sheet.href && foreign(sheet.href)) continue; // Reported as a link above.
    let rules: CSSRuleList;
    try {
      rules = sheet.cssRules;
    } catch {
      problems.push("a stylesheet that cannot be read: " + sheet.href);
      continue;
    }
    for (const rule of Array.from(rules)) {
      for (const url of urls(rule.cssText)) {
        if (foreign(url, sheet.href ?? window.location.href)) problems.push("the stylesheet " + (sheet.href ?? "in the page") + " names " + url);
      }
    }
  }
  return problems;
}

// failedResources: requests from this page that were answered with an HTTP error.
export function failedResources(window: Window): string[] {
  return (window.performance.getEntriesByType("resource") as PerformanceResourceTiming[])
    .filter((entry) => (entry.responseStatus ?? 0) >= 400)
    .map((entry) => entry.name + " answered " + entry.responseStatus);
}
