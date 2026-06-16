FROM node:24-bookworm-slim AS builder

WORKDIR /src

COPY web-admin/package.json web-admin/package-lock.json ./
RUN npm ci

COPY web-admin ./

ARG VITE_ADMIN_API_URL=http://localhost:8082
ENV VITE_ADMIN_API_URL=$VITE_ADMIN_API_URL

RUN npm run build

FROM nginx:1.29-alpine AS runtime

COPY web-admin/nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=builder /src/dist /usr/share/nginx/html

EXPOSE 80

