import Navbar from './components/Navbar'
import { Route, Routes } from 'react-router-dom'
import Home from './components/Home'
import LoginComponent from './components/LoginComponent'
import Error from './components/Error'

function App() {

  return (
    <>
      <div>
         <Navbar/>
         <div className='flex justify-center items-center p-4'>
         <Routes>
           <Route path='/' element={<Home/>}></Route>
           <Route path='login' element={<LoginComponent/>}></Route>
           <Route path='*' element={<Error/>}></Route>

         </Routes>
         </div>
      </div>
    </>
  )
}

export default App
