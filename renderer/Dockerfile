# bookworm: playwright's --with-deps list still resolves fully there
# (trixie dropped ttf-unifont / ttf-ubuntu-font-family → install fails)
FROM python:3.12-slim-bookworm

WORKDIR /app

COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt \
    && playwright install --with-deps chromium

COPY generate.py app.py ./
COPY templates/ ./templates/
COPY fonts/ ./fonts/

EXPOSE 8788
HEALTHCHECK --interval=30s --timeout=5s --start-period=20s \
  CMD python -c "import urllib.request; urllib.request.urlopen('http://127.0.0.1:8788/healthz')" || exit 1

CMD ["uvicorn", "app:app", "--host", "0.0.0.0", "--port", "8788"]
