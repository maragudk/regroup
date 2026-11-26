// Fields are one line tall, and grow with what is written in them. Where the browser can size a field
// to its content, the stylesheet does that. Elsewhere, this sets a field's height from its content when
// the content changes, and when a page comes back with text in a field.
if (!CSS.supports("field-sizing", "content")) {
  const grow = (field) => {
    field.style.height = "auto";
    field.style.height = field.scrollHeight + field.offsetHeight - field.clientHeight + "px";
  };

  document.addEventListener("input", (event) => {
    if (event.target instanceof HTMLTextAreaElement) grow(event.target);
  });

  window.addEventListener("pageshow", () => {
    for (const field of document.querySelectorAll("textarea")) {
      if (field.value !== "" && field.checkVisibility()) grow(field);
    }
  });
}
