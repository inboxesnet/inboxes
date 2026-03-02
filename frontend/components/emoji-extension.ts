import { Extension } from "@tiptap/core";
import { Plugin, PluginKey } from "@tiptap/pm/state";
import { filterEmoji } from "@/lib/emoji-data";

export interface EmojiSuggestionState {
  active: boolean;
  query: string;
  from: number;
  to: number;
  items: { shortcode: string; emoji: string }[];
}

const emojiPluginKey = new PluginKey("emojiSuggestion");

export const EmojiSuggestion = Extension.create<{
  onStateChange: (state: EmojiSuggestionState) => void;
}>({
  name: "emojiSuggestion",

  addOptions() {
    return {
      onStateChange: () => {},
    };
  },

  addProseMirrorPlugins() {
    const { onStateChange } = this.options;

    return [
      new Plugin({
        key: emojiPluginKey,
        view() {
          return {
            update(view) {
              const { state } = view;
              const { selection } = state;

              if (!selection.empty) {
                onStateChange({ active: false, query: "", from: 0, to: 0, items: [] });
                return;
              }

              const pos = selection.$from;
              const textBefore = pos.parent.textBetween(0, pos.parentOffset, undefined, "\ufffc");

              // Find the last `:` that's preceded by whitespace or is at line start
              const match = textBefore.match(/(^|[\s])(:([a-zA-Z0-9_+-]*))$/);
              if (!match) {
                onStateChange({ active: false, query: "", from: 0, to: 0, items: [] });
                return;
              }

              const query = match[3]; // text after the `:`
              const colonOffset = textBefore.length - match[2].length;
              const from = pos.start() + colonOffset;
              const to = pos.start() + textBefore.length;

              const items = filterEmoji(query, 8);

              onStateChange({
                active: true,
                query,
                from,
                to,
                items,
              });
            },
            destroy() {
              onStateChange({ active: false, query: "", from: 0, to: 0, items: [] });
            },
          };
        },
      }),
    ];
  },
});
