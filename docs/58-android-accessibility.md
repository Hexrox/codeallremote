# Android accessibility requirements

## Required behavior

- Every icon-only action has a content description.
- Approval risk, expiry and final result are announced to screen readers.
- Color never carries the only meaning of state or risk.
- Touch targets meet platform guidance and work with large font settings.
- Terminal/output has selectable text and a non-color status representation.

## Testing

Test TalkBack navigation, keyboard/switch access where applicable, high contrast, font scaling, landscape/portrait transitions and reduced-motion settings. Accessibility regressions block Android milestone acceptance for affected screens.

