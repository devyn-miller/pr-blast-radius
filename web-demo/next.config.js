/** @type {import('next').NextConfig} */
const nextConfig = {
  output: 'export',  // Static export for Vercel free tier
  images: {
    unoptimized: true,
  },
}

module.exports = nextConfig
