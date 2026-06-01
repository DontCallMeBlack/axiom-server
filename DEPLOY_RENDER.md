Quick deploy to Render (or any Docker-supporting host)

1. Ensure `frontend/public/loot-images` is committed (run image optimizer locally and commit results).
2. Create a Render account and connect your GitHub repo.
3. Create a new service → "Web Service" → "Deploy from Dockerfile".
4. Set the build and start commands to defaults (the Dockerfile builds and the container runs `./axiom-server`).
5. Add environment variable `MONGO_URI` with your MongoDB Atlas connection string.
6. Deploy; Render will build and run the container. Use the assigned URL to set the frontend's WebSocket URL if needed.

Notes:
- Railway and Fly have similar flows; use their Docker options or a direct Go deploy.
- Vercel: host only the `frontend` folder on Vercel and set backend URL to your Render/Railway service.
