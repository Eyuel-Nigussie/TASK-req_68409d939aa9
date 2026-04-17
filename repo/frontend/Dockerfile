# syntax=docker/dockerfile:1.6
#
# Multi-stage build:
#   * base    — npm install of the dependency tree (cached across runs)
#   * test    — runs `npm test` (targeted by run_tests.sh)
#   * build   — runs `npm run build` and emits the /src/dist bundle
#   * runtime — nginx:alpine serving the bundle + proxying /api/ to the
#               backend service on the compose network

FROM node:20-alpine AS base
WORKDIR /src
COPY package.json package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY . .

# -- test stage --------------------------------------------------------
FROM base AS test
CMD ["npm", "test"]

# -- build stage -------------------------------------------------------
FROM base AS build
RUN npm run build

# -- runtime stage -----------------------------------------------------
FROM nginx:1.27-alpine AS runtime
COPY --from=build /src/dist /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf
EXPOSE 80
