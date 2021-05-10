# FROM registry.sensetime.com/cloudnative4ai/liqingping/mock_nervex:v0.0.1
FROM python:3

WORKDIR /app

COPY requirements.txt ./
RUN pip install --no-cache-dir -r requirements.txt

COPY ./data ./data
COPY ./interaction ./interaction
COPY ./utils ./utils
COPY ./worker ./worker
COPY ./main.py ./main.py

# docker build -t registry.sensetime.com/cloudnative4ai/nervex-mock:v0.0.1 .