using System.Runtime.InteropServices;
using System.Security;

namespace GameLauncher.Authentication;

public static class PasswordBuffer
{
    public static char[] CopyFrom(SecureString securePassword)
    {
        if (securePassword.Length == 0)
        {
            return [];
        }

        IntPtr pointer = IntPtr.Zero;
        try
        {
            pointer = Marshal.SecureStringToGlobalAllocUnicode(securePassword);
            var password = new char[securePassword.Length];
            for (int index = 0; index < password.Length; index++)
            {
                password[index] = (char)Marshal.ReadInt16(pointer, index * sizeof(char));
            }

            return password;
        }
        finally
        {
            if (pointer != IntPtr.Zero)
            {
                Marshal.ZeroFreeGlobalAllocUnicode(pointer);
            }
        }
    }

    public static void Clear(char[] password) => Array.Clear(password, 0, password.Length);
}
