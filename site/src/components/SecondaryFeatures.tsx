'use client'

import { useId } from 'react'
import clsx from 'clsx'

import { Container } from '@/components/Container'

interface Feature {
  name: React.ReactNode
  summary: string
  description: string
  icon: React.ComponentType
}

const features: Array<Feature> = [
  {
    name: 'Developer Productivity',
    summary: 'Save your team tens of hours per month',
    description:
      'Faster development, production debugging, QA testing, pull request reviews, and more. Perfect for large databases.',
    icon: function ProductivityIcon() {
      let id = useId()
      return (
        <>
          <defs>
            <linearGradient
              id={id}
              x1="11.5"
              y1={18}
              x2={36}
              y2="15.5"
              gradientUnits="userSpaceOnUse"
            >
              <stop offset=".194" stopColor="#a78bfa" />
              <stop offset={1} stopColor="#8b5cf6" />
            </linearGradient>
          </defs>
          <path
            d="m30 15-4 5-4-11-4 18-4-11-4 7-4-5"
            stroke={`url(#${id})`}
            strokeWidth={2}
            strokeLinecap="round"
            strokeLinejoin="round"
          />
        </>
      )
    },
  },
  {
    name: 'Self-Hosted & Private',
    summary: 'Your infrastructure, your data, your control',
    description:
      'Deploy on your own AWS infrastructure. All data stays within your environment. No third-party access, ever.',
    icon: function PrivacyIcon() {
      return (
        <>
          <path
            opacity=".5"
            d="M18 6C14.686 6 12 8.686 12 12v6h12v-6c0-3.314-2.686-6-6-6Z"
            fill="#a78bfa"
          />
          <path
            d="M9 18a3 3 0 0 1 3-3h12a3 3 0 0 1 3 3v9a3 3 0 0 1-3 3H12a3 3 0 0 1-3-3v-9Z"
            fill="#8b5cf6"
          />
        </>
      )
    },
  },
  {
    name: 'Cost Efficient',
    summary: 'No SaaS fees, minimal storage overhead',
    description:
      'Pay only for your AWS infrastructure. Copy-on-write snapshots mean branches share unchanged data. A 100GB database with 10 branches might only use 120GB total.',
    icon: function StorageIcon() {
      return (
        <>
          <path
            opacity=".5"
            d="M9 12h18a3 3 0 0 1 3 3v12a3 3 0 0 1-3 3H9a3 3 0 0 1-3-3V15a3 3 0 0 1 3-3Z"
            fill="#a78bfa"
          />
          <path d="M9 6h18a3 3 0 0 1 3 3v3H6V9a3 3 0 0 1 3-3Z" fill="#8b5cf6" />
        </>
      )
    },
  },
  {
    name: 'CI/CD Ready',
    summary: 'Automate database branches in your pipeline',
    description:
      'CLI-first design integrates seamlessly with GitHub Actions, GitLab CI, or any CI/CD tool. Spin up a branch per PR automatically.',
    icon: function CICDIcon() {
      return (
        <>
          <path
            opacity=".5"
            d="M20.793 10.793a1 1 0 0 0-1.414 0L17 13.172l-2.379-2.379a1 1 0 1 0-1.414 1.414l3 3a1 1 0 0 0 1.414 0l3-3a1 1 0 0 0 0-1.414Z"
            fill="#a78bfa"
          />
          <path
            d="M6 8a2 2 0 0 1 2-2h20a2 2 0 0 1 2 2v4a2 2 0 0 1-2 2H8a2 2 0 0 1-2-2V8ZM6 20a2 2 0 0 1 2-2h20a2 2 0 0 1 2 2v4a2 2 0 0 1-2 2H8a2 2 0 0 1-2-2v-4Z"
            fill="#8b5cf6"
          />
        </>
      )
    },
  },
  {
    name: 'Real Production Data',
    summary: 'Test with actual data, not stale fixtures',
    description:
      'Debug issues with real customer data, test migrations against actual schema complexity, and catch edge cases before production.',
    icon: function DataIcon() {
      return (
        <>
          <path
            opacity=".5"
            d="M6 12c0-3.314 5.373-6 12-6s12 2.686 12 6-5.373 6-12 6-12-2.686-12-6Z"
            fill="#a78bfa"
          />
          <path
            d="M6 12v12c0 3.314 5.373 6 12 6s12-2.686 12-6V12"
            fill="#8b5cf6"
          />
        </>
      )
    },
  },
  {
    name: 'Disposable by Design',
    summary: 'Create, destroy, repeat',
    description:
      'Branches are meant to be temporary. Spin them up for testing, tear them down when done. No orphaned databases cluttering your infrastructure.',
    icon: function DisposableIcon() {
      return (
        <>
          <path
            opacity=".5"
            d="M12 6h12a3 3 0 0 1 3 3v15a3 3 0 0 1-3 3H12a3 3 0 0 1-3-3V9a3 3 0 0 1 3-3Z"
            fill="#a78bfa"
          />
          <path
            d="M18 15v9M24 15v9M10.5 6V4.5A1.5 1.5 0 0 1 12 3h12a1.5 1.5 0 0 1 1.5 1.5V6"
            stroke="#8b5cf6"
            strokeWidth={2}
            strokeLinecap="round"
          />
        </>
      )
    },
  },
]

function Feature({
  feature,
  isActive,
  className,
  ...props
}: React.ComponentPropsWithoutRef<'div'> & {
  feature: Feature
  isActive: boolean
}) {
  return (
    <div
      className={clsx(className, !isActive && 'opacity-75 hover:opacity-100')}
      {...props}
    >
      <div
        className={clsx(
          'w-9 rounded-lg',
          isActive ? 'bg-violet-600/20' : 'bg-zinc-800',
        )}
      >
        <svg aria-hidden="true" className="h-9 w-9" fill="none">
          <feature.icon />
        </svg>
      </div>
      <h3
        className={clsx(
          'mt-6 text-xs font-medium tracking-wider uppercase',
          isActive ? 'text-violet-400' : 'text-zinc-400',
        )}
      >
        {feature.name}
      </h3>
      <p className="mt-2 font-display text-lg text-white">{feature.summary}</p>
      <p className="mt-3 text-sm text-zinc-400">{feature.description}</p>
    </div>
  )
}

function FeaturesMobile() {
  return (
    <div className="-mx-4 mt-20 flex flex-col gap-10 overflow-hidden px-4 sm:-mx-6 sm:px-6 lg:hidden">
      {features.map((feature) => (
        <div key={feature.summary}>
          <Feature feature={feature} className="mx-auto max-w-2xl" isActive />
        </div>
      ))}
    </div>
  )
}

function FeaturesDesktop() {
  return (
    <div className="hidden lg:mt-20 lg:block">
      <div className="grid grid-cols-3 gap-8">
        {features.map((feature) => (
          <Feature
            key={feature.summary}
            feature={feature}
            isActive
            className="relative"
          />
        ))}
      </div>
    </div>
  )
}

export function SecondaryFeatures() {
  return (
    <section
      id="secondary-features"
      aria-label="Production ready"
      className="pt-20 pb-14 sm:pt-32 sm:pb-20 lg:pb-32"
      style={{ backgroundColor: '#0a0a0f' }}
    >
      <Container>
        <div className="mx-auto max-w-2xl md:text-center">
          <h2 className="font-display text-2xl tracking-tight text-white sm:text-3xl">
            Why Branchd?
          </h2>
          <p className="mt-4 text-base tracking-tight text-zinc-400">
            Fast, secure, and built for modern development teams
          </p>
        </div>
        <FeaturesMobile />
        <FeaturesDesktop />
      </Container>
    </section>
  )
}
