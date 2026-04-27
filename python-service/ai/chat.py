from openai import OpenAI

client = OpenAI()

def ask(context: str, question: str) -> str:
    response = client.chat.completions.create(
        model="gpt-4o-mini",
        messages=[
            {
                "role": "system",
                "content": (
                    "Você é um assistente especialista em auditoria de joint ventures. "
                    "Responda apenas com base no contexto fornecido. "
                    "Se não houver informação suficiente, diga explicitamente."
                ),
            },
            {
                "role": "user",
                "content": f"Contexto:\n{context}\n\nPergunta: {question}",
            },
        ],
    )
    return response.choices[0].message.content or ""
