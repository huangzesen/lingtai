"""Integration test: real LLM compaction with MiniMax.

Creates a real LLMService + ChatSession with a deliberately small context
window, pumps enough messages to exceed 80% usage, and verifies compaction
fires and produces a working session.

Usage:
    python tests/integration_test_compaction.py
"""

from __future__ import annotations

import os
import sys

# Load .env
from pathlib import Path
env_path = Path(__file__).resolve().parent.parent / ".env"
if env_path.exists():
    for line in env_path.read_text().splitlines():
        line = line.strip()
        if line and not line.startswith("#") and "=" in line:
            k, v = line.split("=", 1)
            v = v.strip().strip("'\"")  # strip surrounding quotes
            os.environ.setdefault(k.strip(), v)

from lingtai.llm.service import LLMService
from lingtai_kernel.llm.interface import TextBlock


def main():
    api_key = os.environ.get("MINIMAX_API_KEY")
    if not api_key:
        print("ERROR: MINIMAX_API_KEY not set in .env or environment")
        sys.exit(1)

    model = "MiniMax-M2.5-highspeed"
    provider = "minimax"

    print(f"Provider: {provider}")
    print(f"Model: {model}")
    print(f"Context window: caller-provided (overridden to 8k for testing)")
    print()

    # Create service
    service = LLMService(
        provider=provider,
        model=model,
        api_key=api_key,
    )

    # Create a session with a deliberately SMALL context window so we can
    # trigger compaction without sending megabytes of text.
    # We'll override context_window after creation.
    chat = service.create_session(
        system_prompt="You are a concise assistant. Reply in 1-2 sentences.",
        model=model,
        thinking="low",
    )

    # Override context window to something tiny (8k tokens)
    # so our test conversation triggers compaction quickly
    FAKE_CTX_WINDOW = 8_000
    # Handle _GatedSession proxy — reach inner session if needed
    inner = getattr(chat, "_inner", chat)
    inner._context_window = FAKE_CTX_WINDOW
    print(f"Context window override: {FAKE_CTX_WINDOW:,} tokens (for testing)")
    print("=" * 60)

    # --- Phase 1: Fill up the context ---
    filler = "Here is some data: " + ("ABCDEF0123456789 " * 100)

    num_turns = 0
    for i in range(15):
        msg = f"Turn {i}: {filler}"
        print(f"\n--- Sending turn {i} ({len(msg)} chars) ---")

        try:
            response = chat.send(msg)
            num_turns += 1
        except Exception as e:
            print(f"  LLM call failed: {e}")
            break

        estimate = chat.interface.estimate_context_tokens()
        ratio = estimate / FAKE_CTX_WINDOW if FAKE_CTX_WINDOW > 0 else 0
        print(f"  Response: {response.text[:80]}...")
        print(f"  Estimated tokens: {estimate:,} / {FAKE_CTX_WINDOW:,} ({ratio:.0%})")

        if ratio > 0.85:
            print(f"\n>>> Context usage {ratio:.0%} exceeds 85% after {num_turns} turns")
            break

    if chat.interface.estimate_context_tokens() / FAKE_CTX_WINDOW <= 0.8:
        print("WARNING: Could not fill context enough to trigger compaction")
        sys.exit(1)

    before_tokens = chat.interface.estimate_context_tokens()
    before_entries = len(chat.interface.conversation_entries())
    print(f"\n{'=' * 60}")
    print(f"BEFORE compaction:")
    print(f"  Entries: {before_entries}")
    print(f"  Estimated tokens: {before_tokens:,}")

    # --- Phase 2: Compact ---
    print(f"\n--- Running compaction (real LLM summarization) ---")

    def summarizer(text: str) -> str:
        target_tokens = int(FAKE_CTX_WINDOW * 0.2)
        prompt = (
            "Summarize the following conversation concisely, preserving key facts and decisions.\n"
            f"Target summary length: ~{target_tokens} tokens.\n\n"
            f"Conversation history:\n{text}"
        )
        response = service.generate(
            prompt,
            temperature=0.1,
            max_output_tokens=target_tokens,
        )
        return response.text.strip() if response and response.text else ""

    new_chat = service.check_and_compact(
        chat,
        summarizer=summarizer,
        threshold=0.8,
    )

    if new_chat is None:
        print("ERROR: check_and_compact returned None — compaction did not trigger")
        sys.exit(1)

    after_tokens = new_chat.interface.estimate_context_tokens()
    after_entries = len(new_chat.interface.conversation_entries())
    reduction = (1 - after_tokens / before_tokens) * 100

    print(f"\nAFTER compaction:")
    print(f"  Entries: {after_entries} (was {before_entries})")
    print(f"  Estimated tokens: {after_tokens:,} (was {before_tokens:,})")
    print(f"  Reduction: {reduction:.0f}%")

    # Show the summary that was generated
    for e in new_chat.interface.entries:
        for b in e.content:
            if isinstance(b, TextBlock) and "[Previous conversation summary]" in b.text:
                summary = b.text.replace("[Previous conversation summary]\n", "")
                print(f"\n  Summary generated by LLM:")
                print(f"  {summary[:500]}")
                break

    # --- Phase 3: Verify the compacted session still works ---
    print(f"\n--- Sending message on compacted session ---")
    try:
        response = new_chat.send("What did we discuss? Summarize briefly.")
        print(f"  Response: {response.text[:300]}")
        print(f"\n{'=' * 60}")
        print("SUCCESS: Compaction worked end-to-end!")
        print(f"  - Filled context to {before_tokens:,} tokens over {num_turns} turns")
        print(f"  - Compacted to {after_tokens:,} tokens ({reduction:.0f}% reduction)")
        print(f"  - Post-compaction session is functional")
    except Exception as e:
        print(f"  ERROR: Post-compaction send failed: {e}")
        sys.exit(1)


if __name__ == "__main__":
    main()
