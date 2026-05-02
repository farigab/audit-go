from fastapi import FastAPI, File, HTTPException, UploadFile

from processors.excel import parse_excel
from processors.pdf import parse_pdf

app = FastAPI(title="audit-go processing service")


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
