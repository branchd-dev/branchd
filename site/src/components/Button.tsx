import Link from 'next/link'
import clsx from 'clsx'

const baseStyles = {
  solid:
    'group inline-flex items-center justify-center rounded-lg py-2.5 px-5 text-sm font-medium focus-visible:outline-2 focus-visible:outline-offset-2',
  outline:
    'group inline-flex ring-1 items-center justify-center rounded-lg py-2.5 px-5 text-sm font-medium',
}

const variantStyles = {
  solid: {
    slate:
      'bg-slate-900 text-white hover:bg-slate-700 hover:text-slate-100 active:bg-slate-800 active:text-slate-300 focus-visible:outline-slate-900',
    blue: 'bg-blue-600 text-white hover:text-slate-100 hover:bg-blue-500 active:bg-blue-800 active:text-blue-100 focus-visible:outline-blue-600',
    white:
      'bg-white text-slate-900 hover:bg-blue-50 active:bg-blue-200 active:text-slate-600 focus-visible:outline-white',
    violet:
      'bg-violet-600 text-white hover:bg-violet-500 active:bg-violet-700 focus-visible:outline-violet-600 shadow-lg shadow-violet-500/50 hover:shadow-xl hover:shadow-violet-500/60 transition-all',
  },
  outline: {
    slate:
      'ring-zinc-700 text-zinc-300 hover:text-white hover:ring-zinc-500 active:ring-zinc-600 active:text-zinc-200 focus-visible:outline-violet-600',
    white:
      'ring-slate-700 text-white hover:ring-slate-500 active:ring-slate-700 active:text-slate-400 focus-visible:outline-white',
  },
}

type ButtonProps = (
  | {
      variant?: 'solid'
      color?: keyof typeof variantStyles.solid
    }
  | {
      variant: 'outline'
      color?: keyof typeof variantStyles.outline
    }
) &
  (
    | Omit<React.ComponentPropsWithoutRef<typeof Link>, 'color'>
    | (Omit<React.ComponentPropsWithoutRef<'button'>, 'color'> & {
        href?: undefined
      })
  )

export function Button({ className, ...props }: ButtonProps) {
  const variant = props.variant ?? 'solid'
  const color = props.color ?? 'violet'

  className = clsx(
    baseStyles[variant],
    variant === 'outline'
      ? variantStyles.outline[color as keyof typeof variantStyles.outline]
      : variant === 'solid'
        ? variantStyles.solid[color as keyof typeof variantStyles.solid]
        : undefined,
    className,
  )

  return typeof props.href === 'undefined' ? (
    <button className={className} {...props} />
  ) : (
    <Link className={className} {...props} />
  )
}
