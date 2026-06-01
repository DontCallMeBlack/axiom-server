Secrets and environment variables (write these down and add them to Vercel Project Settings)

Required:
- MONGO_URI: Your MongoDB Atlas connection string. Example:
  mongodb+srv://<username>:<password>@cluster0.abcdef.mongodb.net/axiom?retryWrites=true&w=majority

Optional but useful:
- VERCEL_TOKEN: Personal Vercel token (only if you want to trigger deploys via API)
- VERCEL_PROJECT_ID: Vercel project id (not required for normal deploys)

How to set in Vercel UI:
1. Go to your project → Settings → Environment Variables
2. Add `MONGO_URI` as a Production/Preview/Development variable depending on environment.
3. Deploy.

Local testing:
- Create a `.env.local` in the `frontend/` folder with:
  MONGO_URI="your-connection-string"
- Then run `npx vercel dev` in `frontend/`.

Security note:
- Do NOT commit your real `MONGO_URI` to the repo. Keep it only in Vercel or your local machine.
