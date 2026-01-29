local HttpService = game:GetService("HttpService")
local Selection = game:GetService("Selection")

local toolbar = plugin:CreateToolbar("GUI Baker")
local button = toolbar:CreateButton("Bake GUI", "Bake selected GUI elements to image", "rbxassetid://105804419332733")

local Serializer = require("@self/Serializer")

local SERVER_URL = "http://localhost:37890/bake"

button.Click:Connect(function()
	local selected = Selection:Get()

	if #selected == 0 then
		warn("GUI Baker: No objects selected!")
		return
	end

	-- Filter for GUI objects only
	local guiObjects = {}
	for _, obj in ipairs(selected) do
		if obj:IsA("GuiObject") then
			table.insert(guiObjects, obj)
		end
	end

	if #guiObjects == 0 then
		warn("GUI Baker: No GUI objects in selection!")
		return
	end

	print("GUI Baker: Serializing " .. #guiObjects .. " objects...")

	local success, result = pcall(function()
		return Serializer.SerializeGuiObjects(guiObjects)
	end)

	if not success then
		warn("GUI Baker: Serialization failed: " .. tostring(result))
		return
	end

	local jsonData = HttpService:JSONEncode(result)

	print("GUI Baker: Sending to baker server...")

	-- Send to local server
	local httpSuccess, httpResult = pcall(function()
		return HttpService:PostAsync(SERVER_URL, jsonData, Enum.HttpContentType.ApplicationJson, false)
	end)

	if httpSuccess then
		print("GUI Baker: ✓ Sent successfully!")
		print("GUI Baker: " .. httpResult)
	else
		warn("GUI Baker: Failed to send to server: " .. tostring(httpResult))
		warn("GUI Baker: Make sure the baker server is running!")
	end
end)
