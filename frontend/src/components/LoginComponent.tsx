import React from 'react'

const LoginComponent = () => {
  return (
    <div className='flex flex-col gap-4 w-1/5'>
      <p className='text-3xl font-semibold text-center'>Sign in</p>
      <div className='flex flex-col gap-3 justify-center border border-rose-700 rounded-2xl p-4 '>
        <input placeholder='Email' type="text" />
        <input placeholder='Username' type="text" />
        <button className='w-full py-3 rounded-2xl bg-rose-700 text-white font-semibold'>Sign up</button>
        <button className='w-full py-3 rounded-2xl bg-rose-700 text-white font-semibold'>Sign up with passkey</button>
      </div>
    </div>

  )
}

export default LoginComponent