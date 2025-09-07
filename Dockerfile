# Используем многоступенчатую сборку
FROM golang:1.24-alpine AS builder

# Устанавливаем зависимости
RUN apk add --no-cache \
    git \
    build-base

# Создаем рабочую директорию
WORKDIR /app

# Копируем файлы модулей
COPY go.mod ./

# Скачиваем зависимости
RUN go mod download

# Копируем остальные файлы
COPY . .

# Собираем приложение
RUN go build -o hexnet_service .

# Финальный образ
FROM alpine:3.18

# Устанавливаем ffmpeg и другие зависимости
RUN apk add --no-cache \
    ca-certificates \
    tzdata

# Копируем бинарник и шаблоны из builder
COPY --from=builder /app/hexnet_service /usr/local/bin/

# Настраиваем переменные среды
ENV UPLOAD_DIR=/hexnet_service \
    PORT=8080

# Открываем порт
EXPOSE 8080

# Запускаем приложение
CMD ["hexnet_service"]
