from fastapi import FastAPI, File, HTTPException, UploadFile
from pydantic import BaseModel, Field

from ai.chat import ask
from processors.excel import parse_excel
from processors.pdf import parse_pdf

app = FastAPI(title="audit-go processing service")


class ChatRequest(BaseModel):
    context: str = Field(default="")
    question: str = Field(min_length=1)
    system_prompt: str = Field(default="")
    user_template: str = Field(default="")
    model: str = Field(default="gpt-4o-mini")
    temperature: float = Field(default=0.2, ge=0, le=2)


@app.get("/health")
async def health() -> dict[str, str]:
    return {"status": "ok"}


@app.post("/parse")
async def parse_document(file: UploadFile = File(...)) -> dict:
    content = await file.read()
    name = file.filename or ""
    lower_name = name.lower()

    if lower_name.endswith(".pdf"):
        result = parse_pdf(content)
    elif lower_name.endswith(".xlsx"):
        result = parse_excel(content)
    else:
        raise HTTPException(status_code=415, detail="unsupported file type")

    return {
        "filename": name,
        "pages": result.get("pages"),
        "text": result.get("text"),
        "markdown": result.get("markdown"),
        "tables": result.get("tables", []),
    }


@app.post("/chat")
async def chat(request: ChatRequest) -> dict[str, str]:
    if not request.context.strip():
        raise HTTPException(status_code=400, detail="context is required")

    answer = ask(
        context=request.context,
        question=request.question,
        system_prompt=request.system_prompt,
        user_template=request.user_template,
        model=request.model,
        temperature=request.temperature,
    )
    return {"answer": answer}
