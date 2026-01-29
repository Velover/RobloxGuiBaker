-- Plugin Script: Extract GuiBase2d Bake Data with JSON Export
-- Recursively extracts properties from selected GuiBase2d instances

local Selection = game:GetService("Selection")
local HttpService = game:GetService("HttpService")

-- JSON Encoding Helper Functions
local function encodeUDim2(udim2)
	return {
		X = { Scale = udim2.X.Scale, Offset = udim2.X.Offset },
		Y = { Scale = udim2.Y.Scale, Offset = udim2.Y.Offset },
	}
end

local function encodeUDim(udim)
	return { Scale = udim.Scale, Offset = udim.Offset }
end

local function encodeVector2(vec2)
	return { X = vec2.X, Y = vec2.Y }
end

local function encodeVector3(vec3)
	return { X = vec3.X, Y = vec3.Y, Z = vec3.Z }
end

local function encodeColor3(color3)
	return { R = color3.R, G = color3.G, B = color3.B }
end

local function encodeNumberSequence(numSeq)
	local keypoints = {}
	for _, keypoint in ipairs(numSeq.Keypoints) do
		table.insert(keypoints, {
			Time = keypoint.Time,
			Value = keypoint.Value,
			Envelope = keypoint.Envelope,
		})
	end
	return { Keypoints = keypoints }
end

local function encodeColorSequence(colorSeq)
	local keypoints = {}
	for _, keypoint in ipairs(colorSeq.Keypoints) do
		table.insert(keypoints, {
			Time = keypoint.Time,
			Value = encodeColor3(keypoint.Value),
		})
	end
	return { Keypoints = keypoints }
end

local function getUIGradientData(instance)
	local uiGradient = instance:FindFirstChildOfClass("UIGradient")
	if uiGradient then
		return {
			Enabled = uiGradient.Enabled,
			Rotation = uiGradient.Rotation,
			Transparency = encodeNumberSequence(uiGradient.Transparency),
			Color = encodeColorSequence(uiGradient.Color),
			Offset = encodeVector2(uiGradient.Offset),
		}
	end
	return nil
end

local function getUIStrokeData(instance)
	local uiStroke = instance:FindFirstChildOfClass("UIStroke")
	if uiStroke then
		return {
			Enabled = uiStroke.Enabled,
			Color = encodeColor3(uiStroke.Color),
			Thickness = uiStroke.Thickness,
			Transparency = uiStroke.Transparency,
			ApplyStrokeMode = uiStroke.ApplyStrokeMode.Name,
			UIGradient = getUIGradientData(uiStroke),
		}
	end
	return nil
end

local function getUICornerData(instance)
	local uiCorner = instance:FindFirstChildOfClass("UICorner")
	if uiCorner then
		return {
			CornerRadius = encodeUDim(uiCorner.CornerRadius),
		}
	end
	return nil
end

local function extractGuiData(instance, parentId)
	if not instance:IsA("GuiBase2d") then
		return nil
	end

	-- Generate unique ID for this instance
	local id = HttpService:GenerateGUID(false)

	local data = {
		Id = id,
		ParentId = parentId or "",
		Name = instance.Name,
		ClassName = instance.ClassName,
		AbsoluteSize = encodeVector2(instance.AbsoluteSize),
		AbsolutePosition = encodeVector2(instance.AbsolutePosition),
		Visible = instance.Visible,
		ZIndex = instance.ZIndex,
	}

	-- GuiObject properties (includes AbsoluteRotation)
	if instance:IsA("GuiObject") then
		data.Size = encodeUDim2(instance.Size)
		data.Position = encodeUDim2(instance.Position)
		data.AnchorPoint = encodeVector2(instance.AnchorPoint)
		data.Rotation = instance.Rotation
		data.AbsoluteRotation = instance.AbsoluteRotation -- Not stackable, just the element's rotation
		data.BackgroundColor3 = encodeColor3(instance.BackgroundColor3)
		data.BackgroundTransparency = instance.BackgroundTransparency
		data.BorderSizePixel = instance.BorderSizePixel
		data.BorderColor3 = encodeColor3(instance.BorderColor3)
		data.LayoutOrder = instance.LayoutOrder

		data.UIGradient = getUIGradientData(instance)
		data.UIStroke = getUIStrokeData(instance)
		data.UICorner = getUICornerData(instance)
	end

	-- CanvasGroup specific (extends GuiObject)
	if instance:IsA("CanvasGroup") then
		data.GroupTransparency = instance.GroupTransparency
		data.GroupColor3 = encodeColor3(instance.GroupColor3)
	end

	-- ImageLabel/ImageButton specific
	if instance:IsA("ImageLabel") or instance:IsA("ImageButton") then
		data.Image = instance.Image
		data.ImageColor3 = encodeColor3(instance.ImageColor3)
		data.ImageTransparency = instance.ImageTransparency
		data.ImageRectOffset = encodeVector2(instance.ImageRectOffset)
		data.ImageRectSize = encodeVector2(instance.ImageRectSize)
		data.ScaleType = instance.ScaleType.Name
	end

	-- TextLabel/TextButton/TextBox specific
	if instance:IsA("TextLabel") or instance:IsA("TextButton") or instance:IsA("TextBox") then
		data.Text = instance.Text
		data.TextColor3 = encodeColor3(instance.TextColor3)
		data.TextTransparency = instance.TextTransparency
		data.TextSize = instance.TextSize
		data.Font = instance.Font.Name
		data.TextXAlignment = instance.TextXAlignment.Name
		data.TextYAlignment = instance.TextYAlignment.Name
		data.TextScaled = instance.TextScaled
		data.TextWrapped = instance.TextWrapped
		data.TextStrokeTransparency = instance.TextStrokeTransparency
		data.TextStrokeColor3 = encodeColor3(instance.TextStrokeColor3)
	end

	-- ScrollingFrame specific
	if instance:IsA("ScrollingFrame") then
		data.CanvasSize = encodeUDim2(instance.CanvasSize)
		data.ScrollBarThickness = instance.ScrollBarThickness
	end

	-- VideoFrame specific
	if instance:IsA("VideoFrame") then
		data.Video = instance.Video
		data.Looped = instance.Looped
		data.Playing = instance.Playing
		data.Volume = instance.Volume
	end

	return data, id
end

local function processInstanceRecursively(instance, dataTable, parentId)
	local guiData, currentId = extractGuiData(instance, parentId)
	if guiData then
		table.insert(dataTable, guiData)

		-- Process children with this element's ID as their parent
		for _, child in ipairs(instance:GetChildren()) do
			processInstanceRecursively(child, dataTable, currentId)
		end
	else
		-- If not a GuiBase2d, still check children but keep same parent
		for _, child in ipairs(instance:GetChildren()) do
			processInstanceRecursively(child, dataTable, parentId)
		end
	end
end

local function gatherBakeData()
	local selectedObjects = Selection:Get()
	local bakeData = {}

	if #selectedObjects == 0 then
		warn("No objects selected!")
		return bakeData
	end

	for _, selectedObject in ipairs(selectedObjects) do
		-- Root elements have no parent (empty string)
		processInstanceRecursively(selectedObject, bakeData, "")
	end

	return bakeData
end

-- Main execution
local data = gatherBakeData()

-- Convert to JSON
local jsonOutput = HttpService:JSONEncode(data)
print("=== JSON OUTPUT ===")
print(jsonOutput)
print("=== END JSON OUTPUT ===")
print(string.format("Exported %d GUI elements", #data))
