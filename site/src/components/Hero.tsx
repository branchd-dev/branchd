'use client'

import { Button } from '@/components/ui/button'
import { Container } from '@/components/Container'
import { useState } from 'react'

export function Hero() {
  const [email, setEmail] = useState('')

  return (
    <div className="relative isolate overflow-hidden">
      <Container className="pt-20 pb-16 lg:pt-32">
        <div className="grid grid-cols-1 items-center gap-12 lg:grid-cols-2 lg:gap-8">
          {/* Left side - Copy */}
          <div>
            <div className="mb-4 flex items-center gap-3">
              <span className="inline-flex items-center rounded-full bg-violet-600/10 px-3 py-1 text-sm font-medium text-violet-400 ring-1 ring-violet-600/20 ring-inset">
                Open Source
              </span>
              <span className="inline-flex items-center rounded-full bg-zinc-800 px-3 py-1 text-sm font-medium text-zinc-300 ring-1 ring-zinc-700 ring-inset">
                Self-host in 5 minutes
              </span>
            </div>
            <h1 className="max-w-2xl font-display text-3xl font-medium tracking-tight text-white sm:text-5xl">
              Instant database branches for PostgreSQL
            </h1>
            <p className="mt-6 max-w-xl text-lg tracking-tight text-zinc-400">
              Open-source database branching for any PostgreSQL setup. Deploy on
              your own infrastructure, keep full control of your data, and
              create instant isolated clones for development, testing,
              production debugging, and more.
            </p>

            {/* CTAs */}
            <div className="mt-8 flex flex-wrap gap-4">
              <Button
                size="default"
                className="h-10 p-6 text-lg whitespace-nowrap"
                asChild
              >
                <a
                  href="https://github.com/branchd-dev/branchd"
                  target="_blank"
                  rel="noopener noreferrer"
                >
                  View on GitHub
                </a>
              </Button>
              <Button
                size="default"
                variant="outline"
                className="h-10 border-zinc-700 bg-transparent p-6 text-lg whitespace-nowrap text-white hover:bg-zinc-800"
                asChild
              >
                <a href="#get-started">Get Started</a>
              </Button>
            </div>
          </div>

          {/* Right side - Video */}
          <div className="relative w-full">
            <div className="relative overflow-hidden rounded-xl border border-zinc-800 bg-zinc-900/50 shadow-2xl shadow-violet-500/10">
              <div
                className="relative w-full"
                style={{ paddingBottom: '60.81%' }}
              >
                <iframe
                  src="https://www.loom.com/embed/786debe116da4ff5af707d80388b4035?sid=786debe116da4ff5af707d80388b4035"
                  frameBorder="0"
                  allowFullScreen
                  className="absolute inset-0 h-full w-full"
                  loading="lazy"
                  title="Branchd Demo"
                ></iframe>
              </div>
            </div>
            {/* Decorative gradient blur */}
            <div className="absolute -inset-4 -z-10 bg-gradient-to-r from-violet-600/20 to-indigo-600/20 opacity-30 blur-3xl" />
          </div>
        </div>
      </Container>
    </div>
  )
}
