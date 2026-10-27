FROM node:24-alpine AS build
WORKDIR /app
RUN corepack enable
COPY package.json pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY . .
RUN pnpm build

FROM node:24-alpine
WORKDIR /app
RUN corepack enable
COPY --from=build /app ./
EXPOSE 4173
CMD ["pnpm", "exec", "vite", "preview", "--host", "0.0.0.0", "--port", "4173"]
