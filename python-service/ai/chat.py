from openai import OpenAI

client = OpenAI()

DEFAULT_SYSTEM_PROMPT = (
    "Voce e um assistente especialista em auditoria de joint ventures. "
    "Responda apenas com base no contexto fornecido. "
    "Se nao houver informacao suficiente, diga explicitamente."
)

DEFAULT_USER_TEMPLATE = "Contexto:\n{{context}}\n\nPergunta: {{question}}"


def render_user_prompt(template: str, context: str, question: str) -> str:
    prompt = template or DEFAULT_USER_TEMPLATE
    return prompt.replace("{{context}}", context).replace("{{question}}", question)


def ask(
    context: str,
    question: str,
    system_prompt: str = DEFAULT_SYSTEM_PROMPT,
    user_template: str = DEFAULT_USER_TEMPLATE,
    model: str = "gpt-4o-mini",
    temperature: float = 0.2,
) -> str:
    response = client.chat.completions.create(
        model=model or "gpt-4o-mini",
        temperature=temperature,
        messages=[
            {
                "role": "system",
                "content": system_prompt or DEFAULT_SYSTEM_PROMPT,
            },
            {
                "role": "user",
                "content": render_user_prompt(user_template, context, question),
            },
        ],
    )
    return response.choices[0].message.content or ""
