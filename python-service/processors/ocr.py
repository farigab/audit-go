import io
import pytesseract
from PIL import Image

def ocr_image(content: bytes, lang: str = "por+eng") -> str:
    image = Image.open(io.BytesIO(content))
    return pytesseract.image_to_string(image, lang=lang)
