import { type Metadata } from 'next'
import { Inter, Lexend } from 'next/font/google'
import clsx from 'clsx'

import '@/styles/tailwind.css'

export const metadata: Metadata = {
  title: {
    template: '%s - Branchd',
    default: 'Branchd - PostgreSQL branching',
  },
  description: 'PostgreSQL branches for your CrunchyBridge cluster',
}

const inter = Inter({
  subsets: ['latin'],
  display: 'swap',
  variable: '--font-inter',
})

const lexend = Lexend({
  subsets: ['latin'],
  display: 'swap',
  variable: '--font-lexend',
})

export default function RootLayout({
  children,
}: {
  children: React.ReactNode
}) {
  return (
    <html
      lang="en"
      className={clsx(
        'h-full scroll-smooth antialiased',
        inter.variable,
        lexend.variable,
      )}
      style={{ backgroundColor: '#0a0a0f' }}
    >
      <body className="flex h-full flex-col" style={{ backgroundColor: '#0a0a0f' }}>{children}</body>
    </html>
  )
}
