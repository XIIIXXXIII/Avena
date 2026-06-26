FROM python:3.11-slim
WORKDIR /app
RUN pip install nats-py
COPY . .
CMD ["python", "main.py"]
