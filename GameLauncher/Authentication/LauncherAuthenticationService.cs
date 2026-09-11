namespace GameLauncher.Authentication;

public sealed class LauncherAuthenticationService : IDisposable
{
    private readonly GameAccountRepository accountRepository;
    private readonly SecureSessionStore sessionStore;
    private readonly AuthTicketService ticketService;
    private AuthenticatedSession? currentSession;

    public LauncherAuthenticationService(
        GameAccountRepository accountRepository,
        SecureSessionStore sessionStore,
        AuthTicketService ticketService)
    {
        this.accountRepository = accountRepository;
        this.sessionStore = sessionStore;
        this.ticketService = ticketService;
    }

    public string? CurrentUsername => currentSession?.Username;
    public bool IsAuthenticated => currentSession is not null;

    public AuthenticationResult RestoreSession()
    {
        AuthenticatedSession? stored = sessionStore.Load();
        if (stored is null)
        {
            return AuthenticationResult.SessionRejected();
        }

        using (stored)
        {
            AuthenticationResult validation = accountRepository.ValidateRememberedSession(
                stored.Username,
                stored.AccountSecret);
            if (!validation.Success)
            {
                sessionStore.Delete();
                return validation;
            }

            SetCurrentSession(validation);
            return validation;
        }
    }

    public AuthenticationResult Login(string username, char[] password, bool remember)
    {
        AuthenticationResult result = accountRepository.Authenticate(username, password);
        if (!result.Success)
        {
            sessionStore.Delete();
            return result;
        }

        SetCurrentSession(result);
        if (remember && currentSession is not null)
        {
            try
            {
                sessionStore.Save(currentSession);
            }
            catch
            {
                Logout();
                throw;
            }
        }
        else
        {
            sessionStore.Delete();
        }

        return result;
    }

    public void ForgetRememberedSession() => sessionStore.Delete();

    public TicketResult CreateLaunchTicket()
    {
        if (currentSession is null)
        {
            return TicketResult.Failed("Sign in before launching the game.");
        }

        AuthenticationResult validation = accountRepository.ValidateRememberedSession(
            currentSession.Username,
            currentSession.AccountSecret);
        if (!validation.Success || validation.AccountSecret is null || validation.Username is null)
        {
            Logout();
            return TicketResult.Failed("Your saved session is no longer valid. Please sign in again.");
        }

        using var validatedSession = new AuthenticatedSession(validation.Username, validation.AccountSecret);
        return TicketResult.Succeeded(ticketService.Create(validatedSession.AccountSecret));
    }

    public void Logout()
    {
        currentSession?.Dispose();
        currentSession = null;
        sessionStore.Delete();
    }

    public void Dispose()
    {
        currentSession?.Dispose();
        currentSession = null;
    }

    private void SetCurrentSession(AuthenticationResult result)
    {
        if (!result.Success || result.Username is null || result.AccountSecret is null)
        {
            throw new InvalidOperationException("Cannot create a session from a failed authentication result.");
        }

        currentSession?.Dispose();
        currentSession = new AuthenticatedSession(result.Username, result.AccountSecret);
    }
}

public sealed record TicketResult(bool Success, string? Token, string? ErrorMessage)
{
    public static TicketResult Succeeded(string token) => new(true, token, null);
    public static TicketResult Failed(string message) => new(false, null, message);
}
