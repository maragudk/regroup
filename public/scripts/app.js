// Fields are one line tall, and grow with what is written in them. Where the browser can size a field
// to its content, the stylesheet does that. Elsewhere, this sets a field's height from its content when
// someone writes in it, when a page comes back with text in a field, and when a thread of comments
// with text in its field opens. The page keeps the height when it puts the same field back.
if (!CSS.supports("field-sizing", "content")) {
  const grow = (field) => {
    field.style.height = "auto";
    field.style.height = field.scrollHeight + field.offsetHeight - field.clientHeight + "px";
  };

  // growAll fields with text in them inside the given element that are shown, and so have a height.
  const growAll = (element) => {
    for (const field of element.querySelectorAll("textarea")) {
      if (field.value !== "" && field.offsetParent !== null) grow(field);
    }
  };

  document.addEventListener("input", (event) => {
    if (event.target instanceof HTMLTextAreaElement) grow(event.target);
  });

  window.addEventListener("pageshow", () => growAll(document));

  // The toggle event does not bubble, so it is caught on its way down.
  document.addEventListener("toggle", (event) => event.target.open && growAll(event.target), true);
}
