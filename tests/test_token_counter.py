from lingtai_kernel.token_counter import count_tokens

def test_count_tokens_returns_positive():
    """count_tokens should return a positive integer for any non-empty string."""
    result = count_tokens("Hello, this is a test string with some tokens.")
    assert isinstance(result, int)
    assert result > 0

def test_count_tokens_empty():
    result = count_tokens("")
    assert result == 0

def test_count_tokens_scales():
    short = count_tokens("hello")
    long = count_tokens("hello " * 100)
    assert long > short
