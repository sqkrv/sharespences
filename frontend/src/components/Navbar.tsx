import React from "react";
import { NavLink } from "react-router-dom";
const Navbar = () => {
  return (
    <div className="w-full flex p-4 px-8 items-center justify-between">
      <p className="font-bold text-3xl tracking-widest w-full">SHARE <span className="text-rose-700">SPENCES</span></p>
      <nav className="w-full  flex gap-4 justify-end font-bold text-3xl">
        <NavLink to="/">
          Home
        </NavLink>
        <NavLink to="/login">
          Login
        </NavLink>
      </nav>
    </div>

  );
};
export default Navbar;