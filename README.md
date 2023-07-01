# PowerShell Utils

## Usage

To run the script in PowerShell on Windows, you must first save the script in a file with the extension .ps1, for example, `CombinePHPFilesToOutput.ps1`.

Then, follow these steps:

1. Open PowerShell. You can do this by searching for "PowerShell" in the start menu.
2. Navigate to the directory where you saved the script. To do this, you can use the cd command. For example, if you saved the file in the `C:\Scripts` directory, you would type cd `C:\Scripts` in the PowerShell console.
3. Before you can run PowerShell scripts, you must change the execution policy to allow local script execution. You can do this with the Set-ExecutionPolicy RemoteSigned command. This command will prompt you for confirmation, to which you should respond with `Y` or `A` for `Yes`. Note that you may need to run PowerShell as administrator to change the execution policy.
4. Finally, you can run the script with the command `.CombinePHPFilesToOutput.ps1`.

_Note that these steps assume that you have the necessary permissions to change the execution policy and that your script is error-free. If you encounter any problems, make sure you are running PowerShell as administrator and that your script is correctly written and saved in the correct location._
