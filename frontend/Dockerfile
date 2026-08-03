FROM node:26-alpine

WORKDIR /app

# Install dependencies first (leverages Docker cache)
COPY package*.json ./
RUN npm install

# Copy the rest of the application
COPY . .

EXPOSE 3000

# Run in development mode to enable hot-reloading
CMD ["npm", "run", "dev"]