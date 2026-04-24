import os

from openai import OpenAI

BASE_URL = os.getenv("OPENAI_BASE_URL", "http://localhost:8091/v1")
API_KEY = os.getenv("OPENAI_API_KEY", "test-token")  # 你的 Bearer token

client = OpenAI(
    base_url=BASE_URL,
    api_key=API_KEY,
)

def pick_model_id() -> str:
    models = client.models.list()
    assert models.object == "list"
    assert len(models.data) >= 1
    return models.data[0].id

def test_models():
    models = client.models.list()
    assert models.object == "list"
    assert len(models.data) >= 1
    print("models.list ok:", [m.id for m in models.data])

def test_chat_non_stream():
    model_id = pick_model_id()
    resp = client.chat.completions.create(
        model=model_id,
        messages=[
            {"role": "system", "content": "You are a helpful assistant."},
            {"role": "user", "content": "Hello!"},
        ],
        temperature=0.7,
        stream=False,
    )
    assert resp.object == "chat.completion"
    assert resp.choices[0].message.role == "assistant"
    print("chat non-stream ok:", resp.id, resp.choices[0].message.content)

def test_chat_stream():
    model_id = pick_model_id()
    stream = client.chat.completions.create(
        model=model_id,
        messages=[
            {"role": "user", "content": "Stream test"},
        ],
        temperature=0.7,
        stream=True,
    )

    text = ""
    first_id = None

    for event in stream:
        # event 是 ChatCompletionChunk
        if first_id is None and getattr(event, "id", None):
            first_id = event.id

        if event.choices and event.choices[0].delta and event.choices[0].delta.content:
            text += event.choices[0].delta.content

    assert first_id is not None
    assert len(text) > 0
    print("chat stream ok:", first_id, text)

if __name__ == "__main__":
    test_models()
    test_chat_non_stream()
    test_chat_stream()
    print("ALL OK")